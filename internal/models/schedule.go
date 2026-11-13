package models

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Schedule is a private, single-day item. Date and clock time use Asia/Shanghai.
type Schedule struct {
	ID       int64   `db:"id" json:"id"`
	Title    string  `db:"title" json:"title"`
	Date     string  `db:"date" json:"date"`
	AllDay   bool    `db:"all_day" json:"allDay"`
	Time     *string `db:"time" json:"time"`
	Location string  `db:"location" json:"location"`
	Notes    string  `db:"notes" json:"notes"`
}

func (s *Schedule) Validate() error {
	s.Title = strings.TrimSpace(s.Title)
	s.Location = strings.TrimSpace(s.Location)
	s.Notes = strings.TrimSpace(s.Notes)
	if s.Title == "" || utf8.RuneCountInString(s.Title) > 100 {
		return fmt.Errorf("标题需为 1—100 字")
	}
	if utf8.RuneCountInString(s.Location) > 200 || utf8.RuneCountInString(s.Notes) > 2000 {
		return fmt.Errorf("地点最多 200 字，备注最多 2000 字")
	}
	d, err := time.Parse("2006-01-02", s.Date)
	if err != nil || d.Format("2006-01-02") != s.Date || d.Year() < 2000 || d.Year() > 2100 {
		return fmt.Errorf("日期需为 2000—2100 年内的 YYYY-MM-DD")
	}
	if s.AllDay {
		s.Time = nil
		return nil
	}
	if s.Time == nil {
		return fmt.Errorf("请选择具体时间")
	}
	t, err := time.Parse("15:04", *s.Time)
	if err != nil || t.Format("15:04") != *s.Time {
		return fmt.Errorf("时间格式需为 HH:mm")
	}
	return nil
}
