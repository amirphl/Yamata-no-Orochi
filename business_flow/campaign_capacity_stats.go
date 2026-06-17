package businessflow

import (
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/amirphl/Yamata-no-Orochi/models"
)

const (
	layer3StatsCSVPath = "docs/src_layer3_stats.csv"
	audienceGradeA     = "A"
	audienceGradeB     = "B"
	audienceGradeC     = "C"
)

type layer3AudienceStats struct {
	BlackUsers uint64
	WhiteUsers uint64
	PinkUsers  uint64

	WeakWhite uint64
	GoodWhite uint64
	BestWhite uint64

	WeakBlack uint64
	GoodBlack uint64
	BestBlack uint64

	WeakPink uint64
	GoodPink uint64
	BestPink uint64
}

type csvCampaignCapacity struct {
	TotalCapacity         uint64
	AudienceGradeCapacity map[string]uint64
}

type layer3AudienceStatsStore struct {
	once sync.Once
	rows map[string]layer3AudienceStats
	err  error
}

var campaignLayer3AudienceStats layer3AudienceStatsStore

func loadLayer3AudienceStats() (map[string]layer3AudienceStats, error) {
	campaignLayer3AudienceStats.once.Do(func() {
		campaignLayer3AudienceStats.rows, campaignLayer3AudienceStats.err = readLayer3AudienceStatsCSV(resolveLayer3StatsCSVPath())
	})
	if campaignLayer3AudienceStats.err != nil {
		return nil, campaignLayer3AudienceStats.err
	}
	return campaignLayer3AudienceStats.rows, nil
}

func resolveLayer3StatsCSVPath() string {
	candidates := []string{
		filepath.Clean(layer3StatsCSVPath),
		filepath.Clean(filepath.Join("..", layer3StatsCSVPath)),
	}

	if _, currentFile, _, ok := runtime.Caller(0); ok {
		baseDir := filepath.Dir(currentFile)
		candidates = append(candidates, filepath.Join(baseDir, "..", layer3StatsCSVPath))
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return filepath.Clean(layer3StatsCSVPath)
}

func readLayer3AudienceStatsCSV(path string) (map[string]layer3AudienceStats, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("layer3 stats csv is empty")
	}

	headerIndex := make(map[string]int, len(records[0]))
	for idx, header := range records[0] {
		headerIndex[normalizeCSVHeader(header)] = idx
	}

	rows := make(map[string]layer3AudienceStats, len(records)-1)
	for _, record := range records[1:] {
		level3 := getCSVValue(record, headerIndex, "layer3_category")
		if level3 == "" {
			continue
		}

		rows[level3] = layer3AudienceStats{
			BlackUsers: mustParseUint64CSVValue(record, headerIndex, "black_users"),
			WhiteUsers: mustParseUint64CSVValue(record, headerIndex, "white_users"),
			PinkUsers:  mustParseUint64CSVValue(record, headerIndex, "pink_users"),
			WeakWhite:  mustParseUint64CSVValue(record, headerIndex, "weak_white"),
			GoodWhite:  mustParseUint64CSVValue(record, headerIndex, "good_white"),
			BestWhite:  mustParseUint64CSVValue(record, headerIndex, "best_white"),
			WeakBlack:  mustParseUint64CSVValue(record, headerIndex, "weak_black"),
			GoodBlack:  mustParseUint64CSVValue(record, headerIndex, "good_black"),
			BestBlack:  mustParseUint64CSVValue(record, headerIndex, "best_black"),
			WeakPink:   mustParseUint64CSVValue(record, headerIndex, "weak_pink"),
			GoodPink:   mustParseUint64CSVValue(record, headerIndex, "good_pink"),
			BestPink:   mustParseUint64CSVValue(record, headerIndex, "best_pink"),
		}
	}

	return rows, nil
}

func normalizeCSVHeader(header string) string {
	return strings.TrimPrefix(strings.TrimSpace(header), "\uFEFF")
}

func getCSVValue(record []string, headerIndex map[string]int, header string) string {
	idx, ok := headerIndex[header]
	if !ok || idx < 0 || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}

func mustParseUint64CSVValue(record []string, headerIndex map[string]int, header string) uint64 {
	value := getCSVValue(record, headerIndex, header)
	if value == "" {
		return 0
	}

	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func calculateCampaignCapacityFromCSV(platform string, level3s []string, grades []string) (*csvCampaignCapacity, bool, error) {
	statsByLevel3, err := loadLayer3AudienceStats()
	if err != nil {
		return nil, false, err
	}

	selectedGrades := campaignAudienceGradesOrDefault(grades)
	result := &csvCampaignCapacity{
		AudienceGradeCapacity: map[string]uint64{
			audienceGradeA: 0,
			audienceGradeB: 0,
			audienceGradeC: 0,
		},
	}

	for _, level3 := range level3s {
		trimmed := strings.TrimSpace(level3)
		if trimmed == "" {
			continue
		}

		row, ok := statsByLevel3[trimmed]
		if !ok {
			return result, false, nil
		}

		result.AudienceGradeCapacity[audienceGradeA] += calculateAudienceGradeCapacity(platform, audienceGradeA, row)
		result.AudienceGradeCapacity[audienceGradeB] += calculateAudienceGradeCapacity(platform, audienceGradeB, row)
		result.AudienceGradeCapacity[audienceGradeC] += calculateAudienceGradeCapacity(platform, audienceGradeC, row)
		result.TotalCapacity += calculateSelectedAudienceGradeCapacity(platform, selectedGrades, row)
	}

	return result, true, nil
}

func calculateAudienceGradeCapacity(platform, grade string, row layer3AudienceStats) uint64 {
	var white uint64
	var pink uint64
	var black uint64

	switch grade {
	case audienceGradeA:
		white = row.BestWhite
		pink = row.BestPink
		black = row.BestBlack
	case audienceGradeB:
		white = row.GoodWhite
		pink = row.GoodPink
		black = row.GoodBlack
	case audienceGradeC:
		white = row.WeakWhite
		pink = row.WeakPink
		black = row.WeakBlack
	default:
		return 0
	}

	if platform == models.CampaignPlatformSMS {
		return white + pink/3
	}
	return white + pink + black
}

func calculateSelectedAudienceGradeCapacity(platform string, grades []string, row layer3AudienceStats) uint64 {
	grades = campaignAudienceGradesOrDefault(grades)

	var total uint64
	for _, grade := range grades {
		total += calculateAudienceGradeCapacity(platform, grade, row)
	}
	return total
}
