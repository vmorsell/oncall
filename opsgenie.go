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
	delay   ogEscalation.EscalationDelay
	periods []Period
}

type Period struct {
	starts time.Time
	ends   time.Time
	email  string
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

	s := Schedule{
		name:  timeline.ScheduleInfo.Name,
		delay: escalationRule.Delay,
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
