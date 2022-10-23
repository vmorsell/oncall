package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	ogAlert "github.com/opsgenie/opsgenie-go-sdk-v2/alert"
	ogClient "github.com/opsgenie/opsgenie-go-sdk-v2/client"
	ogEscalation "github.com/opsgenie/opsgenie-go-sdk-v2/escalation"
	"github.com/opsgenie/opsgenie-go-sdk-v2/og"
	ogSchedule "github.com/opsgenie/opsgenie-go-sdk-v2/schedule"
	ogTeam "github.com/opsgenie/opsgenie-go-sdk-v2/team"
	ogUser "github.com/opsgenie/opsgenie-go-sdk-v2/user"
	"gopkg.in/yaml.v2"
)

type opsGenieClient struct {
	alerts     ogAlert.Client
	escalation ogEscalation.Client
	schedule   ogSchedule.Client
	team       ogTeam.Client
	user       ogUser.Client
}

func NewOpsGenieClient(apiKey string) *opsGenieClient {
	cfg := &ogClient.Config{
		ApiKey: apiKey,
	}

	alert, err := ogAlert.NewClient(cfg)
	if err != nil {
		log.Fatalf("alert: %v", err)
	}
	escalation, err := ogEscalation.NewClient(cfg)
	if err != nil {
		log.Fatalf("escalation: %v", err)
	}
	schedule, err := ogSchedule.NewClient(cfg)
	if err != nil {
		log.Fatalf("schedule: %v", err)
	}
	team, err := ogTeam.NewClient(cfg)
	if err != nil {
		log.Fatalf("team: %v", err)
	}
	user, err := ogUser.NewClient(cfg)
	if err != nil {
		log.Fatalf("user: %v", err)
	}

	return &opsGenieClient{
		*alert,
		*escalation,
		*schedule,
		*team,
		*user,
	}
}

type Escalation struct {
	schedules []Schedule
}

type Schedule struct {
	name    string
	delay   int // Delay in seconds
	periods []Period
}

type Period struct {
	starts time.Time
	ends   time.Time
	email  string
}

// Convert OpsGenie delay to seconds.
func delayToSeconds(v ogEscalation.EscalationDelay) (int, error) {
	mapping := map[og.TimeUnit]int{
		"minutes": 60,
		"hours":   3600,
	}

	unit := string(v.TimeUnit)
	c, ok := mapping[v.TimeUnit]
	if !ok {
		return 0, fmt.Errorf("missing coefficient for unit %s", unit)
	}

	return int(v.TimeAmount) * c, nil
}

func (og *opsGenieClient) getSchedule(ctx context.Context, teamName string, weeks int) (Schedule, error) {
	rules, err := og.team.ListRoutingRules(ctx, &ogTeam.ListRoutingRulesRequest{
		TeamIdentifierType:  ogTeam.Name,
		TeamIdentifierValue: teamName,
	})
	if err != nil {
		return Schedule{}, fmt.Errorf("list routing rules: %w", err)
	}

	if len(rules.RoutingRules) == 0 {
		return Schedule{}, fmt.Errorf("team has no routing rules")
	}

	routingRule := rules.RoutingRules[0]
	escalation, err := og.escalation.Get(ctx, &ogEscalation.GetRequest{
		IdentifierType: ogEscalation.Id,
		Identifier:     routingRule.Notify.Id,
	})
	if err != nil {
		return Schedule{}, fmt.Errorf("get escalation: %w", err)
	}

	// OpsGenie allows multiple escalations per route.
	// Only consider the first escalation rule for now.
	escalationRule := escalation.Rules[0]
	timeline, err := og.schedule.GetTimeline(ctx, &ogSchedule.GetTimelineRequest{
		IdentifierType:  ogSchedule.Id,
		IdentifierValue: escalationRule.Recipient.Id,
		Interval:        weeks,
		IntervalUnit:    ogSchedule.Weeks,
		Date:            ptr(time.Now()),
	})
	if err != nil {
		log.Fatalf("get timeline: %v", err)
	}

	delay, err := delayToSeconds(escalationRule.Delay)
	if err != nil {
		return Schedule{}, fmt.Errorf("delay to seconds: %w", err)
	}

	s := Schedule{
		name:  timeline.ScheduleInfo.Name,
		delay: delay,
	}
	for _, r := range timeline.FinalTimeline.Rotations {
		for _, p := range r.Periods {
			// For some reason, OpsGenie sometimes returns periods before
			// the current time. Ignore those.
			if p.EndDate.Before(time.Now()) {
				continue
			}

			s.periods = append(s.periods, Period{
				starts: p.StartDate,
				ends:   p.EndDate,
				email:  p.Recipient.Name,
			})
		}

	}
	return s, nil
}

type Alert struct {
	created      time.Time
	message        string
	priority     string
	acknowledged bool
	owner        string
}

func (og *opsGenieClient) getAlerts(ctx context.Context, teamName string) ([]Alert, error) {
	res, err := og.alerts.List(ctx, &ogAlert.ListAlertRequest{
		Limit: 20,
		Sort:  ogAlert.CreatedAt,
		Query: fmt.Sprintf(`status:open AND responders: "%s"`, teamName),
	})
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}

	alerts := make([]Alert, 0, len(res.Alerts))
	for _, a := range res.Alerts {
		alerts = append(alerts, Alert{
			created:      a.CreatedAt,
			message:        a.Message,
			priority:     string(a.Priority),
			acknowledged: a.Acknowledged,
			owner:        a.Owner,
		})
	}

	return alerts, nil
}

type Config struct {
	OpsGenie struct {
		APIKey string `yaml:"apiKey"`
	} `yaml:"opsGenie"`
	TeamNames []string `yaml:"teamNames"`
}

func readConfig() (Config, error) {
	home := os.Getenv("HOME")
	src := fmt.Sprintf("%s/.config/oncall/config.yml", home)

	file, err := ioutil.ReadFile(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("no config file found at %s", src)
		}
		return Config{}, fmt.Errorf("read file: %w", err)
	}

	var cfg Config
	err = yaml.Unmarshal(file, &cfg)
	if err != nil {
		return Config{}, fmt.Errorf("unmarshal: %w", err)
	}

	return cfg, nil
}

func ptr[V bool | time.Time](v V) *V {
	return &v
}
