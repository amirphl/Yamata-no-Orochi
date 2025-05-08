// Package services provides external service integrations and technical concerns like notifications and tokens
package services

import (
	"fmt"
	"log"
)

// NotificationService handles sending notifications via SMS and email
type NotificationService interface {
	SendSMS(mobile, message string) error
	SendEmail(email, subject, message string) error
}

// NotificationServiceImpl implements NotificationService
type NotificationServiceImpl struct {
	smsProvider   SMSProvider
	emailProvider EmailProvider
}

// SMSProvider interface for SMS sending
type SMSProvider interface {
	SendSMS(mobile, message string) error
}

// EmailProvider interface for email sending
type EmailProvider interface {
	SendEmail(email, subject, message string) error
}

// NewNotificationService creates a new notification service
func NewNotificationService(smsProvider SMSProvider, emailProvider EmailProvider) NotificationService {
	return &NotificationServiceImpl{
		smsProvider:   smsProvider,
		emailProvider: emailProvider,
	}
}

// SendSMS sends an SMS message to the specified mobile number
func (s *NotificationServiceImpl) SendSMS(mobile, message string) error {
	if s.smsProvider == nil {
		return fmt.Errorf("SMS provider not configured")
	}

	// Validate mobile format
	if len(mobile) != 13 || mobile[:4] != "+989" {
		return fmt.Errorf("invalid mobile number format: %s", mobile)
	}

	return s.smsProvider.SendSMS(mobile, message)
}

// SendEmail sends an email to the specified email address
func (s *NotificationServiceImpl) SendEmail(email, subject, message string) error {
	if s.emailProvider == nil {
		return fmt.Errorf("email provider not configured")
	}

	// Basic email validation
	if len(email) == 0 || !contains(email, "@") {
		return fmt.Errorf("invalid email address: %s", email)
	}

	return s.emailProvider.SendEmail(email, subject, message)
}

type MockSMSProvider struct{}

func NewMockSMSProvider() SMSProvider {
	return &MockSMSProvider{}
}

func (p *MockSMSProvider) SendSMS(mobile, message string) error {
	log.Printf("SMS sent to %s: %s", mobile, message)
	return nil
}

type MockEmailProvider struct{}

func NewMockEmailProvider() EmailProvider {
	return &MockEmailProvider{}
}

func (p *MockEmailProvider) SendEmail(email, subject, message string) error {
	log.Printf("Email sent to %s [%s]: %s", email, subject, message)
	return nil
}

type IranianSMSProvider struct {
	username   string
	password   string
	fromNumber string
}

func NewIranianSMSProvider(username, password, fromNumber string) SMSProvider {
	return &IranianSMSProvider{
		username:   username,
		password:   password,
		fromNumber: fromNumber,
	}
}

func (p *IranianSMSProvider) SendSMS(mobile, message string) error {
	// Implementation would integrate with Iranian SMS providers like:
	// - Kavenegar
	// - SMS.ir
	// - Payamak
	// - etc.

	log.Printf("Sending SMS via Iranian provider to %s: %s", mobile, message)

	// Placeholder implementation
	// In real implementation, make HTTP request to SMS provider API

	return nil
}

type SMTPEmailProvider struct {
	host      string
	port      int
	username  string
	password  string
	fromEmail string
}

func NewSMTPEmailProvider(host string, port int, username, password, fromEmail string) EmailProvider {
	return &SMTPEmailProvider{
		host:      host,
		port:      port,
		username:  username,
		password:  password,
		fromEmail: fromEmail,
	}
}

func (p *SMTPEmailProvider) SendEmail(email, subject, message string) error {
	// Implementation would use net/smtp package or a library like gomail

	log.Printf("Sending email via SMTP to %s [%s]: %s", email, subject, message)

	// Placeholder implementation
	// In real implementation, configure SMTP and send email

	return nil
}

// Helper function
func contains(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
