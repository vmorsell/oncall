package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	ogEscalation "github.com/opsgenie/opsgenie-go-sdk-v2/escalation"
	"github.com/opsgenie/opsgenie-go-sdk-v2/og"
	"github.com/rivo/tview"
)

func onCallScheduleBox(schedule Schedule) *tview.Flex {
	text := tview.NewTextView()
	text.SetDynamicColors(true)
	for _, p := range schedule.periods {
		fmt.Fprintf(text, "%s\n[#aaaaaa]%s -> %s[white]\n\n", emailToName(p.email), fmtTime(p.starts), fmtTime(p.ends))
	}

	flex := tview.NewFlex()
	flex.SetTitle(fmt.Sprintf(" %s: %s ", fmtDelay(schedule.delay), schedule.name))
	flex.SetBorder(true)
	flex.AddItem(text, 0, 1, true)

	return flex
}

func fmtTime(t time.Time) string {
	if t.Before(time.Now()) {
		return "Now"
	}
	return t.Local().Format("Mon Jan _2 15:04")
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

func main() {
	ctx := context.Background()

	cfg, err := readConfig()
	if err != nil {
		log.Fatalf("read config: %v", err)
	}
	og := NewOpsGenieClient(cfg.OpsGenie.APIKey)

	app := tview.NewApplication()

	layout := tview.NewFlex().SetDirection(tview.FlexRow)
	for i, name := range cfg.TeamNames {
		var visible = false
		if i == 0 {
			visible = true
		}

		schedule, err := og.getSchedule(ctx, name, 3)
		if err != nil {
			log.Fatalf("get schedule: %v", err)
		}
		layout.AddItem(onCallScheduleBox(schedule), 0, 1, visible)
	}

	app.SetRoot(layout, true)
	if err := app.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
