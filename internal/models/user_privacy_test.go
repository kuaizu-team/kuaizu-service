package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicUserProjectionDoesNotExposePrivateData(t *testing.T) {
	secret := "private-value"
	nickname := "display-name"
	balance := 10
	user := &User{ID: 7, Nickname: &nickname, Phone: &secret, Email: &secret, WechatID: &secret, AuthImgUrl: &secret, OliveBranchCount: &balance}
	public := user.ToPublicVO()
	encoded, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) || public.OliveBranchCount != nil {
		t.Fatal(string(encoded))
	}
	if public.Nickname == nil || *public.Nickname != nickname {
		t.Fatal("public identity removed")
	}
	if user.Phone == nil || user.ToVO().Phone == nil {
		t.Fatal("owner representation mutated")
	}
}

func TestNestedListsUsePublicUserProjection(t *testing.T) {
	secret := "private-value"
	u := &User{ID: 7, Phone: &secret, Email: &secret, WechatID: &secret, AuthImgUrl: &secret}
	for _, value := range []any{
		(&Project{Creator: u}).ToVO(),
		(ProjectMember{User: u}).ToVO(),
		(&ProjectApplication{Applicant: u}).ToVO(),
		(&OliveBranch{Sender: u, Receiver: u}).ToVO(),
	} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), secret) {
			t.Fatal(string(data))
		}
	}
	detail := (&Project{Creator: u}).ToDetailVO().Creator
	if detail.Phone == nil || detail.Email == nil || detail.Wechat == nil {
		t.Fatal("authorized contact fields lost")
	}
	if detail.AuthImgUrl != nil || detail.OliveBranchCount != nil || detail.FreeBranchUsedToday != nil {
		t.Fatal("project detail exposes non-contact private data")
	}
	if u.ToVO().AuthImgUrl == nil {
		t.Fatal("owner authentication document removed")
	}
}
