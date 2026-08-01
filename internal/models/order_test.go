package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDeliveryIntent(t *testing.T) {
	scene := OrderDeliverySceneEmailPromotion
	payload := `{"scene":"email_promotion","projectId":42,"strategy":"region"}`
	order := &Order{DeliveryScene: &scene, DeliveryPayload: &payload}

	intent, err := order.ParseDeliveryIntent()

	require.NoError(t, err)
	require.NotNil(t, intent)
	assert.Equal(t, scene, intent.Scene)
	require.NotNil(t, intent.ProjectID)
	assert.Equal(t, 42, *intent.ProjectID)
}

func TestParseDeliveryIntentRejectsSceneMismatch(t *testing.T) {
	scene := OrderDeliverySceneSMSNotice
	payload := `{"scene":"email_promotion","projectId":42}`
	order := &Order{DeliveryScene: &scene, DeliveryPayload: &payload}

	intent, err := order.ParseDeliveryIntent()

	assert.Nil(t, intent)
	require.Error(t, err)
}

func TestParseDeliveryIntentAllowsLegacyOrder(t *testing.T) {
	intent, err := (&Order{}).ParseDeliveryIntent()

	require.NoError(t, err)
	assert.Nil(t, intent)
}
