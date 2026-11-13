package handler

import (
	"context"
	"github.com/getkin/kin-openapi/openapi3"
	"testing"
)

func TestScheduleContract(t *testing.T) {
	spec, err := openapi3.NewLoader().LoadFromFile("../../api/service/schedule.openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
}
