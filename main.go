package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func tableBox(title string, headers []string, data [][]string) *tview.Table {
	table := tview.NewTable()
	table.SetBorder(true)
	table.SetTitle(fmt.Sprintf(" %s ", title))

	for i, h := range headers {
		cell := tview.NewTableCell(h)
		cell.SetAttributes(tcell.AttrBold)
		cell.SetSelectable(false)
		cell.SetExpansion(1)

		table.SetCell(0, i, cell)
	}

	for r, d := range data {
		for c, v := range d {
			cell := tview.NewTableCell(v)
			cell.SetMaxWidth(50)

			table.SetCell(r+1, c, cell)
		}
	}

	return table
}

func alertsTable(title string, data []Alert) *tview.Table {
	headers := []string{
		"Created At",
		"Message",
		"Prio",
		"Acked",
		"Owner",
	}

	rows := make([][]string, 0, len(data))
	for _, d := range data {
		// Escape end brackets in message to avoid color changes.
		msg := strings.ReplaceAll(d.message, "]", "[]")

		r := []string{
			fmtTime(d.created),
			msg,
			d.priority,
			fmt.Sprintf("%t", d.acknowledged),
			emailToName(d.owner),
		}
		rows = append(rows, r)
	}

	return tableBox(title, headers, rows)
}

func scheduleTable(title string, data Schedule) *tview.Table {
	headers := []string{
		"Start Time",
		"End Time",
		"Name",
	}

	rows := make([][]string, 0, len(data.periods))
	for _, p := range data.periods {
		r := []string{
			fmtTime(p.starts),
			fmtTime(p.ends),
			emailToName(p.email),
		}
		rows = append(rows, r)
	}

	return tableBox(title, headers, rows)
}

func fmtTime(t time.Time) string {
	now := time.Now()
	if t.After(now.Add(-time.Second)) && t.Before(now) {
		return "Now"
	}
	return t.Local().Format("Mon Jan _2 15:04")
}

func fmtDelay(v int) string {
	return fmt.Sprintf("%d min", v/60)
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

	menu := tview.NewTextView()
	pages := tview.NewPages()

	menu.SetDynamicColors(true)
	menu.SetRegions(true)
	menu.SetHighlightedFunc(func(added, _, _ []string) {
		pages.SwitchToPage(added[0])
	})

	for i, name := range cfg.TeamNames {
		schedule, err := og.getSchedule(ctx, name, 3)
		if err != nil {
			log.Fatalf("get schedule: %v", err)
		}

		alerts, err := og.getAlerts(ctx, name)
		if err != nil {
			log.Fatalf("get alerts: %v", err)
		}

		flex := tview.NewFlex()
		flex.SetDirection(tview.FlexRow)
		flex.AddItem(scheduleTable("Schedule", schedule), 0, 1, true)
		flex.AddItem(alertsTable("Open Alerts", alerts), 0, 1, false)

		pages.AddPage(strconv.Itoa(i), flex, true, i == 0)

		fmt.Fprintf(menu, `%d ["%d"][darkcyan]%s[white][""]  `, i+1, i, name)
	}
	menu.Highlight("0")

	layout := tview.NewFlex().SetDirection(tview.FlexRow)
	layout.AddItem(pages, 0, 1, true)
	layout.AddItem(menu, 1, 1, false)

	app := tview.NewApplication()

	// Listen to keyboard events.
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Page navigation with number keys.
		one := rune(49)
		max := one + rune(pages.GetPageCount()-1)

		if event.Rune() >= one && event.Rune() <= max {
			menu.Highlight(fmt.Sprint(event.Rune() - one)).ScrollToHighlight()
			return nil
		}

		return event
	})

	app.SetRoot(layout, true)
	if err := app.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
