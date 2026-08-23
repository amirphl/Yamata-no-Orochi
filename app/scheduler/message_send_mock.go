package scheduler

import (
	"context"
	"fmt"
	"sync/atomic"
)

const mockSendDescription = "mocked successful send"

var mockMessageIDSequence atomic.Uint64

func nextMockMessageID(provider string) string {
	return fmt.Sprintf("mock-%s-%d", provider, mockMessageIDSequence.Add(1))
}

func maybeMockPayamSMSClient(client PayamSMSClient, enabled bool) PayamSMSClient {
	if !enabled {
		return client
	}
	return &mockPayamSMSClient{PayamSMSClient: client}
}

// mockPayamSMSClient replaces only the provider's send operation. Token and
// status operations continue to use the configured client.
type mockPayamSMSClient struct {
	PayamSMSClient
}

func (c *mockPayamSMSClient) SendBatch(_ context.Context, _ string, items []PayamSMSItem) (PayamSMSSendResult, error) {
	responses := make([]PayamSMSResponseItem, 0, len(items))
	for _, item := range items {
		serverID := nextMockMessageID("sms")
		description := mockSendDescription
		responses = append(responses, PayamSMSResponseItem{
			TrackingID: item.TrackingID,
			Mobile:     item.Recipient,
			ServerID:   &serverID,
			Desc:       &description,
		})
	}
	return PayamSMSSendResult{Items: responses}, nil
}

func maybeMockBaleClient(client BaleClient, enabled bool) BaleClient {
	if !enabled {
		return client
	}
	return &mockBaleClient{BaleClient: client}
}

// mockBaleClient replaces only single and batch sends. Upload and status
// operations continue to use the configured client.
type mockBaleClient struct {
	BaleClient
}

func (c *mockBaleClient) SendMessage(_ context.Context, req *BaleSendMessageRequest) (*BaleSendMessageResponse, error) {
	requestID := ""
	if req != nil {
		requestID = req.RequestID
	}
	return &BaleSendMessageResponse{
		RequestID: requestID,
		MessageID: nextMockMessageID("bale"),
		Provider:  "mock",
	}, nil
}

func (c *mockBaleClient) SendBatch(ctx context.Context, reqs []BaleSendMessageRequest) ([]BaleSendMessageResponse, error) {
	responses := make([]BaleSendMessageResponse, 0, len(reqs))
	for i := range reqs {
		response, err := c.SendMessage(ctx, &reqs[i])
		if err != nil {
			return responses, err
		}
		responses = append(responses, *response)
	}
	return responses, nil
}

func maybeMockRubikaClient(client RubikaClient, enabled bool) RubikaClient {
	if !enabled {
		return client
	}
	return &mockRubikaClient{RubikaClient: client}
}

// mockRubikaClient replaces only bulk sends. Upload and status operations
// continue to use the configured client.
type mockRubikaClient struct {
	RubikaClient
}

func (c *mockRubikaClient) SendBulkMessages(_ context.Context, _ string, messages []RubikaMessagePayload) (*RubikaSendBulkMessagesResponse, error) {
	response := &RubikaSendBulkMessagesResponse{
		Status:     "OK",
		StatusDet:  mockSendDescription,
		HTTPStatus: 200,
	}
	response.Data.MessageStatusList = make([]RubikaMessageStatus, 0, len(messages))
	for _, message := range messages {
		response.Data.MessageStatusList = append(response.Data.MessageStatusList, RubikaMessageStatus{
			MessageID: nextMockMessageID("rubika"),
			Phone:     message.Phone,
			Status:    "sent",
			StatusDet: mockSendDescription,
		})
	}
	return response, nil
}

func maybeMockSplusClient(client SplusClient, enabled bool) SplusClient {
	if !enabled {
		return client
	}
	return &mockSplusClient{SplusClient: client}
}

// mockSplusClient replaces only message sends. Upload and status operations
// continue to use the configured client.
type mockSplusClient struct {
	SplusClient
}

func (c *mockSplusClient) SendMessage(_ context.Context, _ string, req *SplusSendMessageRequest) (*SplusResponse, error) {
	messageID := nextMockMessageID("splus")
	response := &SplusResponse{
		ResultCode:    200,
		ResultMessage: mockSendDescription,
		MessageID:     &messageID,
		HTTPStatus:    200,
	}
	if req != nil && req.UserID != "" {
		userID := req.UserID
		response.UserID = &userID
	}
	return response, nil
}
