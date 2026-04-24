package models

import (
	"github.com/kuaizu-team/kuaizu-service/api"
)

// Major represents a major in the database
type Major struct {
	Id        int    `db:"id"`
	ClassId   int    `db:"class_id"`
	MajorName string `db:"major_name"`
}

// ToVO converts Major to API MajorVO
func (s *Major) ToVO() *api.MajorVO {
	return &api.MajorVO{
		Id:        &s.Id,
		ClassId:   &s.ClassId,
		MajorName: &s.MajorName,
	}
}

// MajorClass represents a major category in the database
type MajorClass struct {
	Id        int     `db:"id"`
	ClassName string  `db:"class_name"`
	Majors    []Major `db:"-"`
}

// ToVO converts MajorClass to API MajorClassVO
func (mc *MajorClass) ToVO() *api.MajorClassVO {
	majors := make([]api.MajorVO, len(mc.Majors))
	for i, m := range mc.Majors {
		majors[i] = *m.ToVO()
	}

	return &api.MajorClassVO{
		Id:        &mc.Id,
		ClassName: &mc.ClassName,
		Majors:    &majors,
	}
}
