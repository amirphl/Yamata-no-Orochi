package businessflow

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/xuri/excelize/v2"
)

// AdminShortLinkFlow provides use cases for admin short link creation
// The admin uploads a CSV with a 'long_link' column and provides a short_link_domain parameter
// For each row with a valid long_link, a short link will be generated and inserted
// The generated short URL format is: <short_link_domain>/s/<uid>
// Note: This flow assumes a public redirect route exists at /s/:uid
// UIDs are generated sequentially from "0000" upward using base36 digits (0-9 then a-z), expanding up to 5 chars (max "zzzzz").
// This function skips rows with empty long_link values
// It returns a summary with counts and the created short links
// Validations are minimal; consumers may validate long_link formats if needed
// Domain should include scheme (https://) if desired by the caller
// If not provided with a scheme, https:// will be prefixed automatically
type AdminShortLinkFlow interface {
	CreateShortLinksFromCSV(ctx context.Context, csvReader io.Reader, shortLinkDomain string, scenarioName string) (*dto.AdminCreateShortLinksResponse, error)
	DownloadShortLinksCSV(ctx context.Context, scenarioID uint) (string, []byte, error)
	DownloadShortLinksWithClicksCSV(ctx context.Context, scenarioID uint) (string, []byte, error)
	DownloadShortLinksWithClicksCSVRange(ctx context.Context, scenarioFrom, scenarioTo uint) (string, []byte, error)
	DownloadShortLinksWithClicksExcelByScenarioNameRegex(ctx context.Context, scenarioNameRegex string) (string, []byte, error)
}

type AdminShortLinkFlowImpl struct {
	repo      repository.ShortLinkRepository
	clickRepo repository.ShortLinkClickRepository
}

func NewAdminShortLinkFlow(repo repository.ShortLinkRepository, clickRepo repository.ShortLinkClickRepository) AdminShortLinkFlow {
	return &AdminShortLinkFlowImpl{repo: repo, clickRepo: clickRepo}
}

func (f *AdminShortLinkFlowImpl) CreateShortLinksFromCSV(ctx context.Context, csvReader io.Reader, shortLinkDomain string, scenarioName string) (*dto.AdminCreateShortLinksResponse, error) {
	if csvReader == nil {
		return nil, NewBusinessError("VALIDATION_ERROR", "CSV file is required", nil)
	}
	shortLinkDomain = normalizeDomain(shortLinkDomain)
	if shortLinkDomain == "" {
		return nil, NewBusinessError("VALIDATION_ERROR", "short_link_domain is required", nil)
	}

	scenarioName = strings.TrimSpace(scenarioName)
	if scenarioName == "" {
		return nil, NewBusinessError("VALIDATION_ERROR", "scenario_name is required", nil)
	}

	reader := csv.NewReader(bufio.NewReader(csvReader))
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		return nil, NewBusinessError("CSV_READ_ERROR", "Failed to read CSV header", err)
	}

	colIndex := map[string]int{}
	for i, h := range header {
		colIndex[strings.ToLower(strings.TrimSpace(h))] = i
	}

	longIdx, ok := colIndex["long_link"]
	if !ok {
		return nil, NewBusinessError("CSV_HEADER_ERROR", "CSV must contain a 'long_link' column", nil)
	}

	lastScenarioID, err := f.repo.GetLastScenarioID(ctx)
	if err != nil {
		return nil, NewBusinessError("FETCH_SCENARIO_ID_FAILED", "Failed to determine next scenario id", err)
	}
	newScenarioID := lastScenarioID + 1

	rows := make([]*models.ShortLink, 0, 256)
	created := 0
	skipped := 0
	var seq uint64
	// Determine starting UID sequence from the highest UID created after the cutoff
	cutoff := time.Date(2025, 11, 10, 15, 45, 11, 401492000, time.UTC)
	lastUID, err := f.repo.GetMaxUIDSince(ctx, cutoff)
	if err != nil {
		return nil, NewBusinessError("FETCH_MAX_UID_FAILED", "Failed to determine highest uid since cutoff", err)
	}
	if lastUID != "" {
		n, err := decodeBase36(lastUID)
		if err != nil {
			return nil, NewBusinessError("INVALID_EXISTING_UID", "Found invalid uid in database", err)
		}
		seq = n + 1
	} else {
		seq = 0
	}
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, NewBusinessError("CSV_READ_ERROR", "Failed to read CSV row", err)
		}
		if longIdx >= len(rec) {
			skipped++
			continue
		}
		longLink := strings.TrimSpace(rec[longIdx])
		if longLink == "" {
			skipped++
			continue
		}

		uid, err := formatSequentialUID(seq)
		if err != nil {
			return nil, NewBusinessError("UID_SEQUENCE_EXHAUSTED", "No more UIDs available up to zzzzz", err)
		}
		seq++
		shortURL := fmt.Sprintf("%s/%s", shortLinkDomain, uid)
		scenarioID := newScenarioID
		rows = append(rows, &models.ShortLink{
			UID:          uid,
			CampaignID:   nil,
			ClientID:     nil,
			PhoneNumber:  nil,
			ScenarioID:   &scenarioID,
			ScenarioName: &scenarioName,
			LongLink:     longLink,
			ShortLink:    shortURL,
		})
		created++
	}

	if len(rows) == 0 {
		return &dto.AdminCreateShortLinksResponse{
			Message:    "No valid rows to create",
			TotalRows:  created + skipped,
			Created:    0,
			Skipped:    skipped,
			ScenarioID: newScenarioID,
		}, nil
	}

	if err := f.repo.SaveBatch(ctx, rows); err != nil {
		return nil, NewBusinessError("CREATE_SHORT_LINKS_FAILED", "Failed to create short links", err)
	}

	return &dto.AdminCreateShortLinksResponse{
		Message:    "Short links created",
		TotalRows:  created + skipped,
		Created:    created,
		Skipped:    skipped,
		ScenarioID: newScenarioID,
	}, nil
}

func normalizeDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return ""
	}
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		domain = "https://" + domain
	}
	// remove trailing slashes
	return strings.TrimRight(domain, "/")
}

func encodeBase36(n uint64) string {
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 16)
	for n > 0 {
		r := n % 36
		buf = append(buf, digits[r])
		n /= 36
	}
	// reverse in place
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

func decodeBase36(s string) (uint64, error) {
	var n uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		var v int
		switch {
		case c >= '0' && c <= '9':
			v = int(c - '0')
		case c >= 'a' && c <= 'z':
			v = int(c-'a') + 10
		case c >= 'A' && c <= 'Z':
			v = int(c-'A') + 10
		default:
			return 0, fmt.Errorf("invalid base36 character: %q", c)
		}
		if v >= 36 {
			return 0, fmt.Errorf("invalid base36 value: %d", v)
		}
		n = n*36 + uint64(v)
	}
	return n, nil
}

func formatSequentialUID(seq uint64) (string, error) {
	s := encodeBase36(seq)
	if len(s) < 4 {
		s = strings.Repeat("0", 4-len(s)) + s
	}
	if len(s) > 5 {
		return "", fmt.Errorf("sequence exhausted at %s", s)
	}
	return s, nil
}

func (f *AdminShortLinkFlowImpl) DownloadShortLinksCSV(ctx context.Context, scenarioID uint) (string, []byte, error) {
	if scenarioID == 0 {
		return "", nil, NewBusinessError("VALIDATION_ERROR", "scenario_id must be greater than 0", nil)
	}

	filter := models.ShortLinkFilter{ScenarioID: &scenarioID}
	rows, err := f.repo.ByFilter(ctx, filter, "id ASC", 0, 0)
	if err != nil {
		return "", nil, NewBusinessError("FETCH_SHORT_LINKS_FAILED", "Failed to fetch short links", err)
	}

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	alreadyFlushed := false
	defer func() {
		if !alreadyFlushed {
			w.Flush()
			alreadyFlushed = true
		}
	}()

	// Header: all current columns in short_links
	header := []string{
		"id",
		"uid",
		"campaign_id",
		"client_id",
		"scenario_id",
		"phone_number",
		"long_link",
		"short_link",
		"created_at",
		"updated_at",
	}
	if err := w.Write(header); err != nil {
		return "", nil, NewBusinessError("CSV_WRITE_ERROR", "Failed to write CSV header", err)
	}

	for _, r := range rows {
		campaignID := ""
		if r.CampaignID != nil {
			campaignID = strconv.FormatUint(uint64(*r.CampaignID), 10)
		}
		clientID := ""
		if r.ClientID != nil {
			clientID = strconv.FormatUint(uint64(*r.ClientID), 10)
		}
		scenario := ""
		if r.ScenarioID != nil {
			scenario = strconv.FormatUint(uint64(*r.ScenarioID), 10)
		}
		phone := ""
		if r.PhoneNumber != nil {
			phone = *r.PhoneNumber
		}

		record := []string{
			strconv.FormatUint(uint64(r.ID), 10),
			r.UID,
			campaignID,
			clientID,
			scenario,
			phone,
			r.LongLink,
			r.ShortLink,
			r.CreatedAt.UTC().Format(time.RFC3339),
			r.UpdatedAt.UTC().Format(time.RFC3339),
		}
		if err := w.Write(record); err != nil {
			return "", nil, NewBusinessError("CSV_WRITE_ERROR", "Failed to write CSV row", err)
		}
	}

	filename := fmt.Sprintf("short_links_scenario_%d.csv", scenarioID)
	if !alreadyFlushed {
		w.Flush()
		alreadyFlushed = true
	}
	return filename, buf.Bytes(), nil
}

func (f *AdminShortLinkFlowImpl) DownloadShortLinksWithClicksCSV(ctx context.Context, scenarioID uint) (string, []byte, error) {
	if scenarioID == 0 {
		return "", nil, NewBusinessError("VALIDATION_ERROR", "scenario_id must be greater than 0", nil)
	}

	rows, err := f.repo.ListWithClicksDetailsByScenario(ctx, scenarioID, "short_links.id ASC, c.id ASC")
	if err != nil {
		return "", nil, NewBusinessError("FETCH_SHORT_LINKS_FAILED", "Failed to fetch short links with clicks", err)
	}

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	alreadyFlushed := false
	defer func() {
		if !alreadyFlushed {
			w.Flush()
			alreadyFlushed = true
		}
	}()

	header := []string{
		"id",
		"uid",
		"campaign_id",
		"client_id",
		"scenario_id",
		"phone_number",
		"long_link",
		"short_link",
		"created_at",
		"updated_at",
		"user_agent",
		"ip",
	}
	if err := w.Write(header); err != nil {
		return "", nil, NewBusinessError("CSV_WRITE_ERROR", "Failed to write CSV header", err)
	}

	for _, r := range rows {
		campaignID := ""
		if r.CampaignID != nil {
			campaignID = strconv.FormatUint(uint64(*r.CampaignID), 10)
		}
		clientID := ""
		if r.ClientID != nil {
			clientID = strconv.FormatUint(uint64(*r.ClientID), 10)
		}
		scenario := ""
		if r.ScenarioID != nil {
			scenario = strconv.FormatUint(uint64(*r.ScenarioID), 10)
		}
		phone := ""
		if r.PhoneNumber != nil {
			phone = *r.PhoneNumber
		}
		ua := ""
		if r.ClickUserAgent != nil {
			ua = *r.ClickUserAgent
		}
		ip := ""
		if r.ClickIP != nil {
			ip = *r.ClickIP
		}

		record := []string{
			strconv.FormatUint(uint64(r.ID), 10),
			r.UID,
			campaignID,
			clientID,
			scenario,
			phone,
			r.LongLink,
			r.ShortLink,
			r.CreatedAt.UTC().Format(time.RFC3339),
			r.UpdatedAt.UTC().Format(time.RFC3339),
			ua,
			ip,
		}
		if err := w.Write(record); err != nil {
			return "", nil, NewBusinessError("CSV_WRITE_ERROR", "Failed to write CSV row", err)
		}
	}

	filename := fmt.Sprintf("short_links_with_clicks_scenario_%d.csv", scenarioID)
	if !alreadyFlushed {
		w.Flush()
		alreadyFlushed = true
	}
	return filename, buf.Bytes(), nil
}

func (f *AdminShortLinkFlowImpl) DownloadShortLinksWithClicksCSVRange(ctx context.Context, scenarioFrom, scenarioTo uint) (string, []byte, error) {
	if scenarioFrom == 0 || scenarioTo == 0 {
		return "", nil, NewBusinessError("VALIDATION_ERROR", "scenario_from and scenario_to must be greater than 0", nil)
	}
	if scenarioTo <= scenarioFrom {
		return "", nil, NewBusinessError("VALIDATION_ERROR", "scenario_to must be greater than scenario_from", nil)
	}

	rows, err := f.repo.ListWithClicksDetailsByScenarioRange(ctx, scenarioFrom, scenarioTo, "short_links.id ASC, c.id ASC")
	if err != nil {
		return "", nil, NewBusinessError("FETCH_SHORT_LINKS_FAILED", "Failed to fetch short links with clicks by range", err)
	}

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	alreadyFlushed := false
	defer func() {
		if !alreadyFlushed {
			w.Flush()
			alreadyFlushed = true
		}
	}()

	header := []string{
		"id",
		"uid",
		"campaign_id",
		"client_id",
		"scenario_id",
		"phone_number",
		"long_link",
		"short_link",
		"created_at",
		"updated_at",
		"user_agent",
		"ip",
	}
	if err := w.Write(header); err != nil {
		return "", nil, NewBusinessError("CSV_WRITE_ERROR", "Failed to write CSV header", err)
	}

	for _, r := range rows {
		campaignID := ""
		if r.CampaignID != nil {
			campaignID = strconv.FormatUint(uint64(*r.CampaignID), 10)
		}
		clientID := ""
		if r.ClientID != nil {
			clientID = strconv.FormatUint(uint64(*r.ClientID), 10)
		}
		scenario := ""
		if r.ScenarioID != nil {
			scenario = strconv.FormatUint(uint64(*r.ScenarioID), 10)
		}
		phone := ""
		if r.PhoneNumber != nil {
			phone = *r.PhoneNumber
		}
		ua := ""
		if r.ClickUserAgent != nil {
			ua = *r.ClickUserAgent
		}
		ip := ""
		if r.ClickIP != nil {
			ip = *r.ClickIP
		}

		record := []string{
			strconv.FormatUint(uint64(r.ID), 10),
			r.UID,
			campaignID,
			clientID,
			scenario,
			phone,
			r.LongLink,
			r.ShortLink,
			r.CreatedAt.UTC().Format(time.RFC3339),
			r.UpdatedAt.UTC().Format(time.RFC3339),
			ua,
			ip,
		}
		if err := w.Write(record); err != nil {
			return "", nil, NewBusinessError("CSV_WRITE_ERROR", "Failed to write CSV row", err)
		}
	}

	filename := fmt.Sprintf("short_links_with_clicks_scenarios_%d_to_%d.csv", scenarioFrom, scenarioTo)
	if !alreadyFlushed {
		w.Flush()
		alreadyFlushed = true
	}
	return filename, buf.Bytes(), nil
}

func (f *AdminShortLinkFlowImpl) DownloadShortLinksWithClicksExcelByScenarioNameRegex(ctx context.Context, scenarioNameRegex string) (string, []byte, error) {
	pattern := strings.TrimSpace(scenarioNameRegex)
	if pattern == "" {
		return "", nil, NewBusinessError("VALIDATION_ERROR", "scenario_name_regex must not be empty", nil)
	}

	rows, err := f.repo.ListWithClicksDetailsByScenarioNameRegex(ctx, pattern, "short_links.scenario_id ASC, short_links.id ASC")
	if err != nil {
		return "", nil, NewBusinessError("FETCH_SHORT_LINKS_FAILED", "Failed to fetch short links by scenario name regex", err)
	}

	// Build Excel with one sheet per scenario
	// Lazy import to avoid unused if excelize not referenced elsewhere
	type excelFile interface{}
	_ = excelFile(nil)

	// Create workbook
	xl := excelize.NewFile()
	defer func() { _ = xl.Close() }()

	// Prepare grouping by scenario
	type rowData = *repository.ShortLinkWithClick
	byScenario := make(map[uint][]rowData)
	nameByScenario := make(map[uint]string)
	order := make([]uint, 0)
	for _, r := range rows {
		if r.ScenarioID == nil {
			continue
		}
		sid := *r.ScenarioID
		byScenario[sid] = append(byScenario[sid], r)
		if _, ok := nameByScenario[sid]; !ok {
			if r.ScenarioName != nil && strings.TrimSpace(*r.ScenarioName) != "" {
				nameByScenario[sid] = *r.ScenarioName
			} else {
				nameByScenario[sid] = fmt.Sprintf("scenario_%d", sid)
			}
			order = append(order, sid)
		}
	}

	// Create sheets
	usedNames := map[string]bool{}
	for i, sid := range order {
		baseName := sanitizeSheetName(nameByScenario[sid])
		name := baseName
		idx := 1
		for usedNames[name] {
			idx++
			name = truncateSheetName(fmt.Sprintf("%s_%d", baseName, idx))
		}
		usedNames[name] = true
		if i == 0 {
			// Rename default sheet
			xl.SetSheetName(xl.GetSheetName(0), name)
		} else {
			_, _ = xl.NewSheet(name)
		}

		header := []string{"id", "uid", "campaign_id", "client_id", "scenario_id", "scenario_name", "phone_number", "long_link", "short_link", "created_at", "updated_at", "user_agent", "ip"}
		_ = xl.SetSheetRow(name, "A1", &header)

		rowsForScenario := byScenario[sid]
		for ri, r := range rowsForScenario {
			campaignID := ""
			if r.CampaignID != nil {
				campaignID = strconv.FormatUint(uint64(*r.CampaignID), 10)
			}
			clientID := ""
			if r.ClientID != nil {
				clientID = strconv.FormatUint(uint64(*r.ClientID), 10)
			}
			scenario := strconv.FormatUint(uint64(sid), 10)
			phone := ""
			if r.PhoneNumber != nil {
				phone = *r.PhoneNumber
			}
			scName := ""
			if r.ScenarioName != nil {
				scName = *r.ScenarioName
			}
			ua := ""
			if r.ClickUserAgent != nil {
				ua = *r.ClickUserAgent
			}
			ip := ""
			if r.ClickIP != nil {
				ip = *r.ClickIP
			}
			record := []string{
				strconv.FormatUint(uint64(r.ID), 10),
				r.UID,
				campaignID,
				clientID,
				scenario,
				scName,
				phone,
				r.LongLink,
				r.ShortLink,
				r.CreatedAt.UTC().Format(time.RFC3339),
				r.UpdatedAt.UTC().Format(time.RFC3339),
				ua,
				ip,
			}
			cellRef, _ := excelize.CoordinatesToCellName(1, ri+2)
			_ = xl.SetSheetRow(name, cellRef, &record)
		}
	}

	buf, err := xl.WriteToBuffer()
	if err != nil {
		return "", nil, NewBusinessError("EXCEL_WRITE_ERROR", "Failed to write Excel file", err)
	}
	filename := "short_links_with_clicks_by_scenario_name.xlsx"
	return filename, buf.Bytes(), nil
}

func sanitizeSheetName(name string) string {
	// Excel sheet names cannot contain: : \\ / ? * [ ] and must be <= 31 chars
	replacer := strings.NewReplacer(":", "_", "\\", "_", "/", "_", "?", "_", "*", "_", "[", "_", "]", "_")
	safe := replacer.Replace(name)
	return truncateSheetName(strings.TrimSpace(safe))
}

func truncateSheetName(name string) string {
	if len(name) > 31 {
		return name[:31]
	}
	if name == "" {
		return "Sheet"
	}
	return name
}
