package businessflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/config"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/redis/go-redis/v9"
)

const audienceSpecCacheTTL = 5 * time.Minute

func audienceSpecPlatformCacheKey(cacheConfig config.CacheConfig, platform string) string {
	return redisKey(cacheConfig, fmt.Sprintf("%s:v3:%s", utils.AudienceSpecCacheKey, platform))
}

func obsoleteAudienceSpecPlatformCacheKeys(cacheConfig config.CacheConfig, platform string) []string {
	return []string{
		redisKey(cacheConfig, fmt.Sprintf("%s:%s", utils.AudienceSpecCacheKey, platform)),
		redisKey(cacheConfig, fmt.Sprintf("%s:v2:%s", utils.AudienceSpecCacheKey, platform)),
	}
}

func normalizeAudienceSpecPlatformRequired(platform string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(platform))
	if p == "" {
		return "", ErrAudienceSpecPlatformRequired
	}
	if !models.IsValidCampaignPlatform(p) {
		return "", ErrAudienceSpecPlatformInvalid
	}
	return p, nil
}

func normalizeAudienceSpecPlatformDefault(platform *string) (string, error) {
	if platform == nil || strings.TrimSpace(*platform) == "" {
		return models.CampaignPlatformSMS, nil
	}
	return normalizeAudienceSpecPlatformRequired(*platform)
}

func audienceCapacityForPlatform(row repository.AudienceSpecSourceRow, platform string) (int, error) {
	if row.DistinctUsers < 0 || row.BlackUsers < 0 || row.WhiteUsers < 0 || row.PinkUsers < 0 ||
		row.WeakWhite < 0 || row.GoodWhite < 0 || row.BestWhite < 0 ||
		row.WeakBlack < 0 || row.GoodBlack < 0 || row.BestBlack < 0 ||
		row.WeakPink < 0 || row.GoodPink < 0 || row.BestPink < 0 || row.ScoredUsers < 0 {
		return 0, fmt.Errorf(
			"negative audience statistics for %s / %s / %s",
			row.Layer1Category,
			row.Layer2Category,
			row.Layer3Category,
		)
	}

	blackUsers := uint64(row.BlackUsers)
	whiteUsers := uint64(row.WhiteUsers)
	pinkUsers := uint64(row.PinkUsers)
	var capacity uint64
	if platform == models.CampaignPlatformSMS {
		capacity = whiteUsers + pinkUsers/3
		if capacity < whiteUsers {
			return 0, fmt.Errorf("SMS audience capacity overflow")
		}
	} else {
		capacity = blackUsers + whiteUsers
		if capacity < blackUsers {
			return 0, fmt.Errorf("audience capacity overflow")
		}
		withPink := capacity + pinkUsers
		if withPink < capacity {
			return 0, fmt.Errorf("audience capacity overflow")
		}
		capacity = withPink
	}

	maxInt := uint64(^uint(0) >> 1)
	if capacity > maxInt {
		return 0, fmt.Errorf("audience capacity %d exceeds the supported integer range", capacity)
	}
	return int(capacity), nil
}

func buildAudienceSpecFromRows(rows []repository.AudienceSpecSourceRow, platform string) (dto.AudienceSpec, error) {
	if _, err := normalizeAudienceSpecPlatformRequired(platform); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("audience specification source is empty")
	}

	type leafAccumulator struct {
		capacity      int
		distinctUsers int64
		blackUsers    int64
		whiteUsers    int64
		pinkUsers     int64
		weakWhite     int64
		goodWhite     int64
		bestWhite     int64
		weakBlack     int64
		goodBlack     int64
		bestBlack     int64
		weakPink      int64
		goodPink      int64
		bestPink      int64
		scoredUsers   int64
		tags          map[uint]struct{}
	}
	accumulators := make(map[string]*leafAccumulator)
	paths := make(map[string][3]string)

	for _, row := range rows {
		level1 := strings.TrimSpace(row.Layer1Category)
		level2 := strings.TrimSpace(row.Layer2Category)
		level3 := strings.TrimSpace(row.Layer3Category)
		if row.TagID == 0 || level1 == "" || level2 == "" || level3 == "" {
			return nil, fmt.Errorf("invalid audience specification source row")
		}
		if !row.StatsFound {
			return nil, fmt.Errorf(
				"no src_layer_all_stats row matches active tag %d at %s / %s / %s",
				row.TagID,
				level1,
				level2,
				level3,
			)
		}
		capacity, err := audienceCapacityForPlatform(row, platform)
		if err != nil {
			return nil, err
		}

		key := level1 + "\x00" + level2 + "\x00" + level3
		leaf := accumulators[key]
		if leaf == nil {
			leaf = &leafAccumulator{
				capacity:      capacity,
				distinctUsers: row.DistinctUsers,
				blackUsers:    row.BlackUsers,
				whiteUsers:    row.WhiteUsers,
				pinkUsers:     row.PinkUsers,
				weakWhite:     row.WeakWhite,
				goodWhite:     row.GoodWhite,
				bestWhite:     row.BestWhite,
				weakBlack:     row.WeakBlack,
				goodBlack:     row.GoodBlack,
				bestBlack:     row.BestBlack,
				weakPink:      row.WeakPink,
				goodPink:      row.GoodPink,
				bestPink:      row.BestPink,
				scoredUsers:   row.ScoredUsers,
				tags:          make(map[uint]struct{}),
			}
			accumulators[key] = leaf
			paths[key] = [3]string{level1, level2, level3}
		} else if leaf.capacity != capacity ||
			leaf.distinctUsers != row.DistinctUsers ||
			leaf.blackUsers != row.BlackUsers ||
			leaf.whiteUsers != row.WhiteUsers ||
			leaf.pinkUsers != row.PinkUsers ||
			leaf.weakWhite != row.WeakWhite ||
			leaf.goodWhite != row.GoodWhite ||
			leaf.bestWhite != row.BestWhite ||
			leaf.weakBlack != row.WeakBlack ||
			leaf.goodBlack != row.GoodBlack ||
			leaf.bestBlack != row.BestBlack ||
			leaf.weakPink != row.WeakPink ||
			leaf.goodPink != row.GoodPink ||
			leaf.bestPink != row.BestPink ||
			leaf.scoredUsers != row.ScoredUsers {
			return nil, fmt.Errorf("conflicting audience statistics for %s / %s / %s", level1, level2, level3)
		}
		leaf.tags[row.TagID] = struct{}{}
	}

	keys := make([]string, 0, len(accumulators))
	for key := range accumulators {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	spec := make(dto.AudienceSpec)
	for _, key := range keys {
		path := paths[key]
		leaf := accumulators[key]
		tagIDs := make([]uint, 0, len(leaf.tags))
		for tagID := range leaf.tags {
			tagIDs = append(tagIDs, tagID)
		}
		sort.Slice(tagIDs, func(i, j int) bool { return tagIDs[i] < tagIDs[j] })
		tags := make([]string, 0, len(tagIDs))
		for _, tagID := range tagIDs {
			tags = append(tags, strconv.FormatUint(uint64(tagID), 10))
		}

		level2Map := spec[path[0]]
		if level2Map == nil {
			level2Map = make(map[string]dto.AudienceSpecLevel2)
			spec[path[0]] = level2Map
		}
		level2Node := level2Map[path[1]]
		if level2Node.Items == nil {
			level2Node.Items = make(map[string]dto.AudienceSpecItem)
		}
		level2Node.Items[path[2]] = dto.AudienceSpecItem{
			Tags:              tags,
			AvailableAudience: leaf.capacity,
			DistinctUsers:     leaf.distinctUsers,
			BlackUsers:        leaf.blackUsers,
			WhiteUsers:        leaf.whiteUsers,
			PinkUsers:         leaf.pinkUsers,
			WeakWhite:         leaf.weakWhite,
			GoodWhite:         leaf.goodWhite,
			BestWhite:         leaf.bestWhite,
			WeakBlack:         leaf.weakBlack,
			GoodBlack:         leaf.goodBlack,
			BestBlack:         leaf.bestBlack,
			WeakPink:          leaf.weakPink,
			GoodPink:          leaf.goodPink,
			BestPink:          leaf.bestPink,
			ScoredUsers:       leaf.scoredUsers,
		}
		level2Map[path[1]] = level2Node
	}
	return spec, nil
}

func (s *CampaignFlowImpl) cachedAudienceSpec(ctx context.Context, platform string) (dto.AudienceSpec, bool) {
	if s.rc == nil {
		return nil, false
	}
	cacheKey := audienceSpecPlatformCacheKey(s.cacheConfig, platform)
	bytes, err := s.rc.Get(ctx, cacheKey).Bytes()
	if err != nil || len(bytes) == 0 {
		return nil, false
	}

	// Reject legacy cache entries with no expiration and any unexpectedly long
	// TTL so every value is guaranteed to be refreshed from PostgreSQL.
	ttl, err := s.rc.TTL(ctx, cacheKey).Result()
	if err != nil || ttl <= 0 || ttl > audienceSpecCacheTTL {
		_ = s.rc.Del(ctx, cacheKey).Err()
		return nil, false
	}

	var spec dto.AudienceSpec
	if err := json.Unmarshal(bytes, &spec); err != nil || validateAudienceSpec(spec, platform) != nil {
		_ = s.rc.Del(ctx, cacheKey).Err()
		return nil, false
	}
	return spec, true
}

func validateAudienceSpec(spec dto.AudienceSpec, platform string) error {
	if len(spec) == 0 {
		return fmt.Errorf("audience specification is empty")
	}
	for level1, level2Map := range spec {
		if strings.TrimSpace(level1) == "" || len(level2Map) == 0 {
			return fmt.Errorf("invalid audience specification level1")
		}
		for level2, node := range level2Map {
			if strings.TrimSpace(level2) == "" || len(node.Items) == 0 {
				return fmt.Errorf("invalid audience specification level2")
			}
			for level3, item := range node.Items {
				if strings.TrimSpace(level3) == "" || len(item.Tags) == 0 || item.AvailableAudience < 0 {
					return fmt.Errorf("invalid audience specification item")
				}
				row := repository.AudienceSpecSourceRow{
					Layer1Category: level1,
					Layer2Category: level2,
					Layer3Category: level3,
					DistinctUsers:  item.DistinctUsers,
					BlackUsers:     item.BlackUsers,
					WhiteUsers:     item.WhiteUsers,
					PinkUsers:      item.PinkUsers,
					WeakWhite:      item.WeakWhite,
					GoodWhite:      item.GoodWhite,
					BestWhite:      item.BestWhite,
					WeakBlack:      item.WeakBlack,
					GoodBlack:      item.GoodBlack,
					BestBlack:      item.BestBlack,
					WeakPink:       item.WeakPink,
					GoodPink:       item.GoodPink,
					BestPink:       item.BestPink,
					ScoredUsers:    item.ScoredUsers,
				}
				expectedCapacity, err := audienceCapacityForPlatform(row, platform)
				if err != nil || expectedCapacity != item.AvailableAudience {
					return fmt.Errorf("invalid audience specification statistics")
				}
				seenTags := make(map[uint64]struct{}, len(item.Tags))
				for _, tag := range item.Tags {
					tagID, err := strconv.ParseUint(strings.TrimSpace(tag), 10, 64)
					if err != nil || tagID == 0 {
						return fmt.Errorf("invalid audience specification tag")
					}
					if _, duplicate := seenTags[tagID]; duplicate {
						return fmt.Errorf("duplicate audience specification tag")
					}
					seenTags[tagID] = struct{}{}
				}
			}
		}
	}
	return nil
}

// ListAudienceSpec returns a platform-specific spec from the five-minute cache,
// rebuilding it exclusively from PostgreSQL on every cache miss.
func (s *CampaignFlowImpl) ListAudienceSpec(ctx context.Context, platform *string) (*dto.ListAudienceSpecResponse, error) {
	normalizedPlatform, err := normalizeAudienceSpecPlatformDefault(platform)
	if err != nil {
		return nil, NewBusinessError("LIST_AUDIENCE_SPEC_PLATFORM_INVALID", "Invalid platform", err)
	}
	hideTestLayer := s.shouldHideTestAudience(ctx)

	if cached, ok := s.cachedAudienceSpec(ctx, normalizedPlatform); ok {
		if hideTestLayer {
			cached = filterAudienceSpecLayer(cached, "L1-test")
		}
		return &dto.ListAudienceSpecResponse{
			Message: "Audience spec retrieved from cache",
			Spec:    cached,
		}, nil
	}

	rows, err := s.audienceSpecRepo.ListActive(ctx)
	if err != nil {
		return nil, NewBusinessError("LIST_AUDIENCE_SPEC_DATABASE_FAILED", "Failed to load audience spec from database", err)
	}
	spec, err := buildAudienceSpecFromRows(rows, normalizedPlatform)
	if err != nil {
		return nil, NewBusinessError("LIST_AUDIENCE_SPEC_BUILD_FAILED", "Failed to build audience spec", err)
	}

	if s.rc != nil {
		if bytes, marshalErr := json.Marshal(spec); marshalErr == nil {
			cacheKey := audienceSpecPlatformCacheKey(s.cacheConfig, normalizedPlatform)
			_ = s.rc.Del(ctx, obsoleteAudienceSpecPlatformCacheKeys(s.cacheConfig, normalizedPlatform)...).Err()
			_ = s.rc.Set(ctx, cacheKey, bytes, audienceSpecCacheTTL).Err()
		}
	}
	if hideTestLayer {
		spec = filterAudienceSpecLayer(spec, "L1-test")
	}
	return &dto.ListAudienceSpecResponse{
		Message: "Audience spec rebuilt from database",
		Spec:    spec,
	}, nil
}

func invalidateAudienceSpecCache(ctx context.Context, client *redis.Client, cacheConfig config.CacheConfig, platform string) error {
	if client == nil {
		return nil
	}
	keys := append(
		[]string{audienceSpecPlatformCacheKey(cacheConfig, platform)},
		obsoleteAudienceSpecPlatformCacheKeys(cacheConfig, platform)...,
	)
	return client.Del(ctx, keys...).Err()
}
