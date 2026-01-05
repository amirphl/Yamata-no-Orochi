package businessflow

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"runtime"
	"strconv"
	"strings"
	"sync"
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

	// Determine new scenario id upfront for the response (with lock)
	lockShortLinkGen()
	lastScenarioID, err := f.repo.GetLastScenarioID(ctx)
	if err != nil {
		unlockShortLinkGen()
		return nil, NewBusinessError("FETCH_SCENARIO_ID_FAILED", "Failed to determine next scenario id", err)
	}
	newScenarioID := lastScenarioID + 1
	unlockShortLinkGen()

	// Buffer the CSV content to allow async processing after we return
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, csvReader); err != nil {
		return nil, NewBusinessError("CSV_READ_ERROR", "Failed to read CSV", err)
	}

	// Spawn background job with longer timeout
	go func(data []byte, domain, scenario string, scenarioID uint) {
		lockShortLinkGen()
		defer unlockShortLinkGen()

		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		reader := csv.NewReader(bufio.NewReader(bytes.NewReader(data)))
		reader.TrimLeadingSpace = true

		header, err := reader.Read()
		if err != nil {
			return
		}
		colIndex := map[string]int{}
		for i, h := range header {
			colIndex[strings.ToLower(strings.TrimSpace(h))] = i
		}
		longIdx, ok := colIndex["long_link"]
		if !ok {
			return
		}

		// Compute starting UID sequence
		cutoff := time.Date(2025, 11, 10, 15, 45, 11, 401492000, time.UTC)
		lastUID, err := f.repo.GetMaxUIDSince(bgCtx, cutoff)
		if err != nil {
			return
		}
		var seq uint64
		if lastUID != "" {
			n, err := decodeBase36(lastUID)
			if err != nil {
				return
			}
			seq = n + 1
		} else {
			seq = 0
		}

		batch := make([]*models.ShortLink, 0, 5000)
		flush := func() {
			if len(batch) == 0 {
				return
			}
			_ = f.repo.SaveBatch(bgCtx, batch)
			batch = batch[:0]
		}
		for {
			rec, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			if longIdx >= len(rec) {
				continue
			}
			longLink := strings.TrimSpace(rec[longIdx])
			if longLink == "" {
				continue
			}

			uid, err := formatSequentialUID(seq)
			if err != nil {
				break
			}
			seq++
			shortURL := fmt.Sprintf("%s/%s", domain, uid)
			sid := scenarioID
			sn := scenario
			batch = append(batch, &models.ShortLink{
				UID:          uid,
				CampaignID:   nil,
				ClientID:     nil,
				PhoneNumber:  nil,
				ScenarioID:   &sid,
				ScenarioName: &sn,
				LongLink:     longLink,
				ShortLink:    shortURL,
			})
			if len(batch) >= 5000 {
				flush()
			}
		}
		flush()
	}(buf.Bytes(), shortLinkDomain, scenarioName, newScenarioID)

	// Return immediately with accepted message and the scenario id
	return &dto.AdminCreateShortLinksResponse{
		Message:    "Upload accepted; processing asynchronously",
		TotalRows:  0,
		Created:    0,
		Skipped:    0,
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

	rows, err := f.repo.ListWithClicksDetailsByScenario(ctx, scenarioID, "short_link_id ASC, id ASC")
	if err != nil {
		return "", nil, NewBusinessError("FETCH_SHORT_LINKS_FAILED", "Failed to fetch short links with clicks", err)
	}

	records := buildClickRecords(rows, buildClickRecord)

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

	for _, record := range records {
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

	rows, err := f.repo.ListWithClicksDetailsByScenarioRange(ctx, scenarioFrom, scenarioTo, "short_link_id ASC, id ASC")
	if err != nil {
		return "", nil, NewBusinessError("FETCH_SHORT_LINKS_FAILED", "Failed to fetch short links with clicks by range", err)
	}

	records := buildClickRecords(rows, buildClickRecord)

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

	for _, record := range records {
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

	// rows, err := f.repo.ListWithClicksDetailsByScenarioNameRegex(ctx, pattern, "scenario_id ASC, short_link_id ASC, id ASC")
	rows, err := f.repo.ListWithClicksDetailsByScenarioNameLike(ctx, pattern, "scenario_id ASC, short_link_id ASC, id ASC")
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
		records := buildClickRecords(rowsForScenario, func(r *repository.ShortLinkWithClick) []string {
			return buildClickRecordWithScenario(sid, nameByScenario[sid], r)
		})
		for ri, record := range records {
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

func buildClickRecords(rows []*repository.ShortLinkWithClick, builder func(*repository.ShortLinkWithClick) []string) [][]string {
	if len(rows) == 0 {
		return [][]string{}
	}
	out := make([][]string, len(rows))
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}
	if workers > len(rows) {
		workers = len(rows)
	}
	chunk := (len(rows) + workers - 1) / workers

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		start := w * chunk
		if start >= len(rows) {
			break
		}
		end := start + chunk
		if end > len(rows) {
			end = len(rows)
		}
		wg.Add(1)
		go func(s, e int) {
			defer wg.Done()
			for i := s; i < e; i++ {
				out[i] = builder(rows[i])
			}
		}(start, end)
	}
	wg.Wait()
	return out
}

func buildClickRecord(r *repository.ShortLinkWithClick) []string {
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

	return []string{
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
}

func buildClickRecordWithScenario(sid uint, scenarioName string, r *repository.ShortLinkWithClick) []string {
	// Reuse base builder but override scenario id/name to avoid nil checks
	record := buildClickRecord(r)
	record[4] = strconv.FormatUint(uint64(sid), 10)
	record[5] = scenarioName
	return record
}
