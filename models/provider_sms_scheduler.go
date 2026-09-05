package models

// The provider-specific scheduler records intentionally have distinct Go
// types and table names. Their fields mirror the previous generic SMS records
// where the transport contract is shared, while gateway ownership is encoded
// by the type/table instead of a mutable provider discriminator.
type PayamProcessedCampaign ProcessedCampaign

func (PayamProcessedCampaign) TableName() string { return "payam_processed_campaigns" }

type CandooProcessedCampaign ProcessedCampaign

func (CandooProcessedCampaign) TableName() string { return "candoo_processed_campaigns" }

type PayamSentSMS SentSMS

func (PayamSentSMS) TableName() string { return "payam_sent_sms" }

type CandooSentSMS SentSMS

func (CandooSentSMS) TableName() string { return "candoo_sent_sms" }

type PayamSMSStatusJob CampaignStatusJob

func (PayamSMSStatusJob) TableName() string { return "payam_sms_status_jobs" }

type CandooSMSStatusJob CampaignStatusJob

func (CandooSMSStatusJob) TableName() string { return "candoo_sms_status_jobs" }

type PayamSMSStatusResult SMSStatusResult

func (PayamSMSStatusResult) TableName() string { return "payam_sms_status_results" }

type CandooSMSStatusResult SMSStatusResult

func (CandooSMSStatusResult) TableName() string { return "candoo_sms_status_results" }

type PayamSMSSendAttempt SMSProviderSendAttempt

func (PayamSMSSendAttempt) TableName() string { return "payam_sms_send_attempts" }

type CandooSMSSendAttempt SMSProviderSendAttempt

func (CandooSMSSendAttempt) TableName() string { return "candoo_sms_send_attempts" }
