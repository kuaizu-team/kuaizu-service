package models

import "time"

type SchedulePrivacy struct {
	TeamDetails    bool `db:"team_details" json:"teamDetails"`
	MemberContacts bool `db:"member_contacts" json:"memberContacts"`
}
type SchedulePerson struct {
	ID        int    `db:"id" json:"id"`
	Name      string `db:"name" json:"name"`
	ProfileID int    `db:"profile_id" json:"profileId"`
	Phone     string `db:"phone" json:"phone,omitempty"`
	Wechat    string `db:"wechat" json:"wechat,omitempty"`
	Email     string `db:"email" json:"email,omitempty"`
}
type ScheduleShare struct {
	ID      int64  `db:"id" json:"id"`
	Token   string `db:"token" json:"token"`
	OwnerID int    `db:"owner_id" json:"ownerId"`
	Kind    string `db:"kind" json:"kind"`
	Date    string `db:"share_date" json:"date"`
	Payload string `db:"payload" json:"-"`
}
type ScheduleNotice struct {
	ID      int64  `db:"id" json:"id"`
	JointID int64  `db:"joint_id" json:"jointId"`
	Message string `db:"message" json:"message"`
	Read    bool   `db:"is_read" json:"read"`
}

// ScheduleWindow uses half-open intervals; old point items occupy one minute.
func ScheduleWindow(s Schedule) (time.Time, time.Time) {
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	if s.AllDay {
		a, _ := time.ParseInLocation("2006-01-02", s.Date, zone)
		return a, a.AddDate(0, 0, 1)
	}
	clock := "00:00"
	if s.Time != nil {
		clock = *s.Time
	}
	a, _ := time.ParseInLocation("2006-01-02 15:04", s.Date+" "+clock, zone)
	b := a.Add(time.Minute)
	if s.EndDate != nil && s.EndTime != nil {
		b, _ = time.ParseInLocation("2006-01-02 15:04", *s.EndDate+" "+*s.EndTime, zone)
	}
	return a, b
}
func SchedulesOverlap(a, b Schedule) bool {
	as, ae := ScheduleWindow(a)
	bs, be := ScheduleWindow(b)
	return as.Before(be) && bs.Before(ae)
}
