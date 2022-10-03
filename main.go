package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	ogAlert "github.com/opsgenie/opsgenie-go-sdk-v2/alert"
	ogClient "github.com/opsgenie/opsgenie-go-sdk-v2/client"
	ogEscalation "github.com/opsgenie/opsgenie-go-sdk-v2/escalation"
	"github.com/opsgenie/opsgenie-go-sdk-v2/og"
	ogSchedule "github.com/opsgenie/opsgenie-go-sdk-v2/schedule"
	ogTeam "github.com/opsgenie/opsgenie-go-sdk-v2/team"
	ogUser "github.com/opsgenie/opsgenie-go-sdk-v2/user"
	"github.com/rivo/tview"
	"gopkg.in/yaml.v2"
)

type opsGenie struct {
	alerts     *ogAlert.Client
	escalation *ogEscalation.Client
	schedule   *ogSchedule.Client
	team       *ogTeam.Client
	user       *ogUser.Client
}

func NewOpsGenie(apiKey string) opsGenie {
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

	return opsGenie{
		alert,
		escalation,
		schedule,
		team,
		user,
	}
}

func (og opsGenie) teamOnCallUsers(ctx context.Context, teamID string) (string, error) {
	rules, err := og.team.ListRoutingRules(ctx, &ogTeam.ListRoutingRulesRequest{
		TeamIdentifierValue: teamID,
		TeamIdentifierType:  ogTeam.Id,
	})
	if err != nil {
		return "", fmt.Errorf("get routing rule: %w", err)
	}

	return fmt.Sprintf("%#v", rules.RoutingRules), nil
}

type Escalation struct {
	schedules []Schedule
}

type Schedule struct {
	name            string
	delay           ogEscalation.EscalationDelay
	onCallUsers     []OnCallUser
	nextOnCallUsers []OnCallUser
}

type OnCallUser struct {
	name       string
	employeeID string
	starts     time.Time
	ends       time.Time
}

func onCallUsers(ctx context.Context, app *tview.Application, og opsGenie, teamName string) *tview.Flex {
	flex := tview.NewFlex()
	flex.SetDirection(tview.FlexRow)

	routingRules, err := og.team.ListRoutingRules(ctx, &ogTeam.ListRoutingRulesRequest{
		TeamIdentifierType:  ogTeam.Name,
		TeamIdentifierValue: teamName,
	})
	if err != nil {
		log.Fatalf("list routing rules: %v", err)
	}

	if len(routingRules.RoutingRules) == 0 {
		log.Fatalf("no routing rules found")
	}

	escalationID := routingRules.RoutingRules[0].Notify.Id

	escalation, err := og.escalation.Get(ctx, &ogEscalation.GetRequest{
		IdentifierType: ogEscalation.Id,
		Identifier:     escalationID,
	})
	if err != nil {
		log.Fatalf("get escalation: %v", err)
	}

	var data Escalation

	for _, r := range escalation.Rules {
		timeline, err := og.schedule.GetTimeline(ctx, &ogSchedule.GetTimelineRequest{
			IdentifierType:  ogSchedule.Id,
			IdentifierValue: r.Recipient.Id,
			Interval:        3,
			IntervalUnit:    ogSchedule.Weeks,
			Date:            ptr(time.Now()),
		})
		if err != nil {
			log.Fatalf("get timeline: %v", err)
		}

		s := Schedule{
			name:  timeline.ScheduleInfo.Name,
			delay: r.Delay,
		}
		for _, r := range timeline.FinalTimeline.Rotations {
			usersSeen := make(map[string]struct{}, 2)
			for _, p := range r.Periods {
				// Display the current user and the next, independently of
				// weird period cut ups from OpsGenie.

				if p.EndDate.Before(time.Now()) {
					// Previous user.
					continue
				}

				if _, ok := usersSeen[p.Recipient.Id]; !ok {
					usersSeen[p.Recipient.Id] = struct{}{}
				}

				// Break the loop when we have seen the full schedules of
				// the current and the next user.
				if len(usersSeen) > 2 {
					break
				}

				user, err := og.user.Get(ctx, &ogUser.GetRequest{
					Identifier: p.Recipient.Id,
				})
				if err != nil {
					log.Fatalf("get user: %v", err)
				}

				var employeeID string
				if eids, ok := user.Details["employeenumber"]; ok {
					if len(eids) == 1 {
						employeeID = eids[0]
					}
				}

				u := OnCallUser{
					name:       p.Recipient.Name,
					employeeID: employeeID,
					starts:     p.StartDate,
					ends:       p.EndDate,
				}

				if p.StartDate.Before(time.Now()) {
					s.onCallUsers = append(s.onCallUsers, u)
				} else {
					s.nextOnCallUsers = append(s.nextOnCallUsers, u)
				}
			}
		}
		data.schedules = append(data.schedules, s)
	}

	for _, s := range data.schedules {
		f := tview.NewFlex()
		f.SetTitle(fmt.Sprintf("%s: %s", fmtDelay(s.delay), s.name))
		f.SetBorder(true)

		current := tview.NewTextView()
		current.SetChangedFunc(func() {
			app.Draw()
		})
		current.SetDynamicColors(true)
		for _, u := range s.onCallUsers {
			fmt.Fprintf(current, "%s (%s)\n[#aaaaaa]now -> %s[white]\n\n", emailToName(u.name), u.employeeID, u.ends.Local().Format("mon jan 2 15.04"))
		}

		next := tview.NewTextView()
		next.SetChangedFunc(func() {
			app.Draw()
		})
		next.SetDynamicColors(true)
		for _, u := range s.nextOnCallUsers {
			fmt.Fprintf(next, "[#aaaaaa]%s (%s)\n%s -> %s[white]\n\n", emailToName(u.name), u.employeeID, u.starts.Local().Format("Mon Jan _2 15:04"), u.ends.Local().Format("Mon Jan 2 15.04"))
		}

		f.AddItem(current, 0, 1, false)
		f.AddItem(next, 0, 1, false)

		height := 2 + 3*max(len(s.onCallUsers), len(s.nextOnCallUsers))

		flex.AddItem(f, height, 0, false)
	}

	return flex
}

func max(v ...int) int {
	if len(v) == 0 {
		panic("no values provided")
	}
	if len(v) == 1 {
		return v[0]
	}

	out := v[0]
	for _, vv := range v[1:] {
		if vv > out {
			out = vv
		}
	}
	return out
}

func fmtDelay(v ogEscalation.EscalationDelay) string {
	units := map[og.TimeUnit]string{
		"minutes": "min",
		"hours":   "h",
	}

	unit := string(v.TimeUnit)
	if res, ok := units[v.TimeUnit]; ok {
		unit = res
	}

	return fmt.Sprintf("%d %s", v.TimeAmount, unit)
}

func emailToName(v string) string {
	v = strings.Split(v, "@")[0]
	v = strings.ReplaceAll(v, ".", " ")
	v = strings.Title(v)
	return v
}

type Config struct {
	OpsGenie struct {
		APIKey string `yaml:"apiKey"`
	} `yaml:"opsGenie"`
	TeamNames []string `yaml:"teamNames"`
}

func main() {
	ctx := context.Background()

	var cfg Config
	home := os.Getenv("HOME")
	src := fmt.Sprintf("%s/.config/oncall/config.yml", home)

	file, err := ioutil.ReadFile(src)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			panic(err)
		}
	}

	err = yaml.Unmarshal(file, &cfg)
	if err != nil {
		panic(err)
	}

	og := NewOpsGenie(cfg.OpsGenie.APIKey)

	app := tview.NewApplication()

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(onCallUsers(ctx, app, og, cfg.TeamNames[0]), 0, 1, true)
	flex.AddItem(tview.NewBox().SetBorder(true).SetTitle(" Open Alerts "), 0, 3, false)

	app.SetRoot(flex, true)
	if err := app.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}

func ptr[V bool | time.Time](v V) *V { return &v }
