package repository

import (
	"testing"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

func TestSMSStatusResultWriteBatchStaysBelowPostgresBindLimit(t *testing.T) {
	t.Parallel()

	statement := &gorm.Statement{DB: newAudienceProfileDryRunDB(t)}
	if err := statement.Parse(&models.SMSStatusResult{}); err != nil {
		t.Fatalf("parse SMS status result schema: %v", err)
	}

	// Use every model field as a conservative upper bound. GORM omits some
	// generated/default fields, so the real parameter count is lower.
	const postgresMaxBindParameters = 65_535
	upperBound := smsStatusResultWriteBatchSize * len(statement.Schema.DBNames)
	if upperBound >= postgresMaxBindParameters {
		t.Fatalf("SMS status batch may use %d bind parameters; must remain below %d", upperBound, postgresMaxBindParameters)
	}
}
