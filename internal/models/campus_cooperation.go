package models

import (
	"encoding/json"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/oss"
)

const (
	MsgBizKeyCampusCooperationResult      = "MSG_CAMPUS_COOPERATION_RESULT"
	StatusNotificationCooperationApproved = "cooperation-approved"
	StatusNotificationCooperationRejected = "cooperation-rejected"
)

type CommercialOrder struct {
	ID          int64     `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	CompanyName string    `db:"company_name" json:"companyName"`
	Description *string   `db:"description" json:"description"`
	Status      string    `db:"status" json:"status"`
	CreatedBy   *int      `db:"created_by" json:"createdBy"`
	UpdatedBy   *int      `db:"updated_by" json:"updatedBy"`
	CreatedAt   time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt   time.Time `db:"updated_at" json:"updatedAt"`
}

type CooperationLink struct {
	Title  string `json:"title"`
	URL    string `json:"url"`
	Public *bool  `json:"public,omitempty"` // Missing on legacy data: inherit links_visible.
}

type CooperationContact struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Value string `json:"value"`
}

type CampusCooperation struct {
	ImageURLs           []string             `db:"-" json:"imageUrls"`
	ID                  int64                `db:"id" json:"id"`
	CommercialOrderID   int64                `db:"commercial_order_id" json:"commercialOrderId"`
	SchoolID            int                  `db:"school_id" json:"schoolId"`
	ClaimedBy           *int                 `db:"claimed_by" json:"claimedBy"`
	ClaimedAt           time.Time            `db:"claimed_at" json:"claimedAt"`
	Title               *string              `db:"title" json:"title"`
	Introduction        *string              `db:"introduction" json:"introduction"`
	TagsJSON            *string              `db:"tags" json:"-"`
	Tags                []string             `db:"-" json:"tags"`
	ViewCount           int64                `db:"view_count" json:"viewCount"`
	LinksVisible        bool                 `db:"links_visible" json:"linksVisible"`
	LinksJSON           *string              `db:"links" json:"-"`
	ImagesVisible       bool                 `db:"images_visible" json:"imagesVisible"`
	ImageTitle          *string              `db:"image_title" json:"imageTitle"`
	ImagesJSON          *string              `db:"images" json:"-"`
	ContactsVisible     bool                 `db:"contacts_visible" json:"contactsVisible"`
	ContactsJSON        *string              `db:"contacts" json:"-"`
	ApplicationRequired bool                 `db:"application_required" json:"applicationRequired"`
	Published           bool                 `db:"published" json:"published"`
	UpdatedBy           *int                 `db:"updated_by" json:"updatedBy"`
	UpdatedAt           time.Time            `db:"updated_at" json:"updatedAt"`
	OrderName           string               `db:"order_name" json:"orderName"`
	CompanyName         string               `db:"company_name" json:"companyName"`
	SchoolName          string               `db:"school_name" json:"schoolName"`
	Links               []CooperationLink    `db:"-" json:"links"`
	Images              []string             `db:"-" json:"images"`
	Contacts            []CooperationContact `db:"-" json:"contacts"`
}

func (c *CampusCooperation) DecodeContent() error {
	c.Tags = []string{}
	c.Links = []CooperationLink{}
	c.Images = []string{}
	c.Contacts = []CooperationContact{}
	for _, field := range []struct {
		raw    *string
		target interface{}
	}{
		{c.TagsJSON, &c.Tags}, {c.LinksJSON, &c.Links}, {c.ImagesJSON, &c.Images}, {c.ContactsJSON, &c.Contacts},
	} {
		if field.raw != nil {
			if err := json.Unmarshal([]byte(*field.raw), field.target); err != nil {
				return err
			}
		}
	}
	if c.Tags == nil {
		c.Tags = []string{}
	}
	if c.Links == nil {
		c.Links = []CooperationLink{}
	}
	if c.Images == nil {
		c.Images = []string{}
	}
	if c.Contacts == nil {
		c.Contacts = []CooperationContact{}
	}
	c.ImageURLs = make([]string, 0, len(c.Images))
	for _, key := range c.Images {
		c.ImageURLs = append(c.ImageURLs, oss.FullURL(key))
	}
	return nil
}

type CooperationApplication struct {
	ID               int64           `db:"id" json:"id"`
	CooperationID    int64           `db:"cooperation_id" json:"cooperationId"`
	UserID           int             `db:"user_id" json:"userId"`
	AttemptNo        int             `db:"attempt_no" json:"attemptNo"`
	Status           string          `db:"status" json:"status"`
	ProfileJSON      *string         `db:"profile_snapshot" json:"-"`
	ProfileSnapshot  json.RawMessage `db:"-" json:"profileSnapshot,omitempty"`
	AppliedAt        time.Time       `db:"applied_at" json:"appliedAt"`
	ReviewedBy       *int            `db:"reviewed_by" json:"reviewedBy"`
	ReviewedAt       *time.Time      `db:"reviewed_at" json:"reviewedAt"`
	ReviewRemark     *string         `db:"review_remark" json:"reviewRemark,omitempty"`
	CooperationTitle *string         `db:"cooperation_title" json:"cooperationTitle"`
	CompanyName      string          `db:"company_name" json:"companyName"`
	SchoolName       string          `db:"school_name" json:"schoolName"`
}
