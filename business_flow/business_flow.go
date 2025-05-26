// Package businessflow contains the business logic for the application.
package businessflow

import (
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
)

const RequestIDKey = "X-Request-ID"

// ClientMetadata holds all client-related information for audit logging and session tracking
type ClientMetadata struct {
	IPAddress  string            `json:"ip_address"`
	UserAgent  string            `json:"user_agent"`
	DeviceInfo map[string]string `json:"device_info,omitempty"`
	Location   *LocationInfo     `json:"location,omitempty"`
	RequestID  string            `json:"request_id,omitempty"`
	SessionID  string            `json:"session_id,omitempty"`
	Additional map[string]string `json:"additional,omitempty"`
}

// LocationInfo holds geographical location information
type LocationInfo struct {
	Country   string `json:"country,omitempty"`
	Region    string `json:"region,omitempty"`
	City      string `json:"city,omitempty"`
	Latitude  string `json:"latitude,omitempty"`
	Longitude string `json:"longitude,omitempty"`
}

// NewClientMetadata creates a new ClientMetadata instance with basic information
func NewClientMetadata(ipAddress, userAgent string) *ClientMetadata {
	return &ClientMetadata{
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		DeviceInfo: make(map[string]string),
		Additional: make(map[string]string),
	}
}

// AddDeviceInfo adds device information to the metadata
func (cm *ClientMetadata) AddDeviceInfo(key, value string) {
	if cm.DeviceInfo == nil {
		cm.DeviceInfo = make(map[string]string)
	}
	cm.DeviceInfo[key] = value
}

// AddAdditional adds additional custom information to the metadata
func (cm *ClientMetadata) AddAdditional(key, value string) {
	if cm.Additional == nil {
		cm.Additional = make(map[string]string)
	}
	cm.Additional[key] = value
}

// SetLocation sets location information
func (cm *ClientMetadata) SetLocation(location *LocationInfo) {
	cm.Location = location
}

// SetRequestID sets the request ID
func (cm *ClientMetadata) SetRequestID(requestID string) {
	cm.RequestID = requestID
}

// SetSessionID sets the session ID
func (cm *ClientMetadata) SetSessionID(sessionID string) {
	cm.SessionID = sessionID
}

// ToAuthCustomerDTO converts a customer model to AuthCustomerDTO for authentication responses
func ToAuthCustomerDTO(customer models.Customer) dto.AuthCustomerDTO {
	dto := dto.AuthCustomerDTO{
		ID:                      customer.ID,
		UUID:                    customer.UUID.String(),
		Email:                   customer.Email,
		RepresentativeFirstName: customer.RepresentativeFirstName,
		RepresentativeLastName:  customer.RepresentativeLastName,
		RepresentativeMobile:    customer.RepresentativeMobile,
		AccountType:             customer.AccountType.DisplayName,
		CompanyName:             customer.CompanyName,
		IsActive:                customer.IsActive,
		IsEmailVerified:         customer.IsEmailVerified,
		IsMobileVerified:        customer.IsMobileVerified,
		CreatedAt:               customer.CreatedAt.Format(time.RFC3339),
		ReferrerAgencyID:        customer.ReferrerAgencyID,
	}

	return dto
}

func ToCustomerSessionDTO(session models.CustomerSession) dto.CustomerSessionDTO {
	return dto.CustomerSessionDTO{
		SessionToken: session.SessionToken,
		RefreshToken: session.RefreshToken,
		ExpiresIn:    int(session.ExpiresAt.Sub(time.Now()).Seconds()), // TODO: UTC?
		TokenType:    "Bearer",
		CreatedAt:    session.CreatedAt.Format(time.RFC3339),
	}
}
