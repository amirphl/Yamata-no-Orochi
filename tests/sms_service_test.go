package tests

import (
	"context"
	"testing"

	"github.com/amirphl/Yamata-no-Orochi/app/services"
	"github.com/stretchr/testify/assert"
)

func TestSMSService(t *testing.T) {
	// Test mock SMS service
	mockSMS := services.NewMockSMSService()

	ctx := context.Background()
	customerID := int64(123)

	// Test sending SMS
	err := mockSMS.SendSMS(ctx, "989123456789", "Test message", &customerID)
	assert.NoError(t, err)

	// Check if message was recorded
	messages := mockSMS.(*services.MockSMSService).GetSentMessages()
	assert.Len(t, messages, 1)
	assert.Equal(t, "989123456789", messages[0].Recipient)
	assert.Equal(t, "Test message", messages[0].Message)
	assert.Equal(t, &customerID, messages[0].CustomerID)

	// Test sending OTP
	err = mockSMS.SendOTP(ctx, "989123456789", "123456", &customerID)
	assert.NoError(t, err)

	// Check if OTP was recorded
	messages = mockSMS.(*services.MockSMSService).GetSentMessages()
	assert.Len(t, messages, 2)
	assert.Equal(t, "123456", messages[1].Message)

	// Test clearing messages
	mockSMS.(*services.MockSMSService).ClearSentMessages()
	messages = mockSMS.(*services.MockSMSService).GetSentMessages()
	assert.Len(t, messages, 0)
}

func TestNotificationServiceWithSMS(t *testing.T) {
	// Test notification service with SMS
	mockSMS := services.NewMockSMSService()
	mockEmail := services.NewMockEmailProvider()

	notificationSvc := services.NewNotificationService(mockSMS, mockEmail)

	ctx := context.Background()
	customerID := int64(123)

	// Test sending SMS via notification service
	err := notificationSvc.SendSMS(ctx, "+989123456789", "Test notification", &customerID)
	assert.NoError(t, err)

	// Check if message was sent (should be converted from +989 to 989 format)
	messages := mockSMS.(*services.MockSMSService).GetSentMessages()
	assert.Len(t, messages, 1)
	assert.Equal(t, "989123456789", messages[0].Recipient)
	assert.Equal(t, "Test notification", messages[0].Message)

	// Test invalid mobile number
	err = notificationSvc.SendSMS(ctx, "+98912345678", "Test", &customerID) // Too short
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid mobile number format")

	err = notificationSvc.SendSMS(ctx, "+9891234567890", "Test", &customerID) // Too long
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid mobile number format")

	err = notificationSvc.SendSMS(ctx, "+989123456789", "Test", &customerID) // Valid
	assert.NoError(t, err)
}
