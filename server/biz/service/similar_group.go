package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"

	"gorm.io/gorm"
)

// previewLimit 相似单词组列表卡片上最多显示几个词
const previewLimit = 3

var ErrSimilarGroupInvalid = errors.New("invalid similar group request")

type ensureSimilarGroupWordFunc func(
	context.Context,
	string,
	EnsureOptions,
) (int64, EnsureAction, error)

type mergeSimilarGroupFunc func(context.Context, []int64) (int64, MergeAction, error)

type loadSimilarGroupFunc func(context.Context, int64) (*model.SimilarGroupDetailData, error)

// AddWordToSimilarGroup initializes both words when necessary, merges their
// memberships, and returns the final group. Repeated calls are idempotent.
func AddWordToSimilarGroup(
	ctx context.Context,
	word string,
	candidateWord string,
) (*model.SimilarGroupDetailData, error) {
	return addWordToSimilarGroup(
		ctx,
		word,
		candidateWord,
		EnsureWordFullyFavorited,
		MergeGroup,
		GetSimilarGroupDetail,
	)
}

func addWordToSimilarGroup(
	ctx context.Context,
	word string,
	candidateWord string,
	ensure ensureSimilarGroupWordFunc,
	merge mergeSimilarGroupFunc,
	load loadSimilarGroupFunc,
) (*model.SimilarGroupDetailData, error) {
	word = strings.ToLower(strings.TrimSpace(word))
	candidateWord = strings.ToLower(strings.TrimSpace(candidateWord))
	if word == candidateWord ||
		!associationWordPattern.MatchString(word) ||
		!associationWordPattern.MatchString(candidateWord) {
		return nil, ErrSimilarGroupInvalid
	}

	started := time.Now()
	wordID, wordAction, err := ensure(ctx, word, EnsureOptions{})
	if err != nil {
		log.Printf("[group-add] initialize target failed word=%q candidate=%q err=%v elapsed=%s",
			word, candidateWord, err, time.Since(started))
		return nil, fmt.Errorf("initialize target word: %w", err)
	}
	candidateID, candidateAction, err := ensure(ctx, candidateWord, EnsureOptions{})
	if err != nil {
		log.Printf("[group-add] initialize candidate failed word=%q candidate=%q err=%v elapsed=%s",
			word, candidateWord, err, time.Since(started))
		return nil, fmt.Errorf("initialize candidate word: %w", err)
	}

	groupID, mergeAction, err := merge(ctx, []int64{wordID, candidateID})
	if err != nil {
		log.Printf("[group-add] merge failed word=%q word_id=%d candidate=%q candidate_id=%d err=%v elapsed=%s",
			word, wordID, candidateWord, candidateID, err, time.Since(started))
		return nil, fmt.Errorf("merge similar group: %w", err)
	}
	data, err := load(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("load merged similar group: %w", err)
	}
	if data == nil {
		return nil, errors.New("merged similar group not found")
	}
	log.Printf(
		"[group-add] ok word=%q word_id=%d word_action=%s candidate=%q candidate_id=%d candidate_action=%s group_id=%d merge_action=%s members=%d elapsed=%s",
		word,
		wordID,
		wordAction,
		candidateWord,
		candidateID,
		candidateAction,
		groupID,
		mergeAction,
		len(data.Members),
		time.Since(started),
	)
	return data, nil
}

// GetSimilarGroupByWord returns the single group containing word.
func GetSimilarGroupByWord(ctx context.Context, word string) (*model.SimilarGroupDetailData, error) {
	if !db.Enabled() {
		log.Printf("[group-by-word] db disabled word=%q", word)
		return nil, db.ErrDBDisabled
	}

	normalized := strings.ToLower(strings.TrimSpace(word))
	var member db.WordGroupMember
	if err := db.DB.WithContext(ctx).
		Table("word_group_members AS members").
		Select("members.*").
		Joins("JOIN words ON words.id = members.word_id").
		Where("words.word = ?", normalized).
		Take(&member).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		log.Printf("[group-by-word] find member failed word=%q err=%v", normalized, err)
		return nil, err
	}

	return GetSimilarGroupDetail(ctx, member.GroupID)
}

// ListSimilarGroups 返回全部相似单词组，按 updated_at DESC 排序；不支持任何过滤参数。
func ListSimilarGroups(ctx context.Context) (*model.SimilarGroupListData, error) {
	if !db.Enabled() {
		log.Printf("[groups] db disabled")
		return nil, db.ErrDBDisabled
	}

	var groups []db.WordGroup
	if err := db.DB.WithContext(ctx).Order("updated_at DESC").Find(&groups).Error; err != nil {
		log.Printf("[groups] find groups failed err=%v", err)
		return nil, err
	}
	data := &model.SimilarGroupListData{Items: []model.SimilarGroupSummary{}, Total: int64(len(groups))}
	if len(groups) == 0 {
		return data, nil
	}

	groupIDs := make([]int64, 0, len(groups))
	for _, g := range groups {
		groupIDs = append(groupIDs, g.ID)
	}
	var members []db.WordGroupMember
	if err := db.DB.WithContext(ctx).
		Where("group_id IN ?", groupIDs).
		Order("group_id, id").Find(&members).Error; err != nil {
		log.Printf("[groups] find members failed group_ids=%v err=%v", groupIDs, err)
		return nil, err
	}

	// 按 group_id 收集前 previewLimit 个 word_id
	previewWordIDs := make(map[int64][]int64, len(groupIDs))
	allWordIDs := make([]int64, 0, len(members))
	for _, m := range members {
		if len(previewWordIDs[m.GroupID]) < previewLimit {
			previewWordIDs[m.GroupID] = append(previewWordIDs[m.GroupID], m.WordID)
			allWordIDs = append(allWordIDs, m.WordID)
		}
	}

	wordByID := make(map[int64]string, len(allWordIDs))
	if len(allWordIDs) > 0 {
		var words []db.Word
		if err := db.DB.WithContext(ctx).
			Select("id, word").
			Where("id IN ?", allWordIDs).Find(&words).Error; err != nil {
			log.Printf("[groups] find preview words failed word_ids=%v err=%v", allWordIDs, err)
			return nil, err
		}
		for _, w := range words {
			wordByID[w.ID] = w.Word
		}
	}

	for _, g := range groups {
		preview := make([]string, 0, previewLimit)
		for _, wid := range previewWordIDs[g.ID] {
			if s := wordByID[wid]; s != "" {
				preview = append(preview, s)
			}
		}
		data.Items = append(data.Items, model.SimilarGroupSummary{
			ID:          g.ID,
			MemberCount: g.MemberCount,
			Preview:     preview,
			UpdatedAt:   g.UpdatedAt.Format(time.RFC3339),
		})
	}
	return data, nil
}

// GetSimilarGroupDetail 返回指定组的成员详情；组不存在返回 (nil, nil)。
func GetSimilarGroupDetail(ctx context.Context, id int64) (*model.SimilarGroupDetailData, error) {
	if !db.Enabled() {
		log.Printf("[group-detail] db disabled id=%d", id)
		return nil, db.ErrDBDisabled
	}
	var g db.WordGroup
	if err := db.DB.WithContext(ctx).Where("id = ?", id).Take(&g).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		log.Printf("[group-detail] take group failed id=%d err=%v", id, err)
		return nil, err
	}

	var members []db.WordGroupMember
	if err := db.DB.WithContext(ctx).
		Where("group_id = ?", g.ID).
		Order("id").Find(&members).Error; err != nil {
		log.Printf("[group-detail] find members failed groupID=%d err=%v", g.ID, err)
		return nil, err
	}

	data := &model.SimilarGroupDetailData{
		ID:          g.ID,
		MemberCount: g.MemberCount,
		Members:     []model.SimilarGroupMember{},
		UpdatedAt:   g.UpdatedAt.Format(time.RFC3339),
	}
	if len(members) == 0 {
		return data, nil
	}

	wordIDs := make([]int64, 0, len(members))
	for _, m := range members {
		wordIDs = append(wordIDs, m.WordID)
	}
	var words []db.Word
	if err := db.DB.WithContext(ctx).Where("id IN ?", wordIDs).Find(&words).Error; err != nil {
		log.Printf("[group-detail] find words failed groupID=%d word_ids=%v err=%v", g.ID, wordIDs, err)
		return nil, err
	}
	wordByID := make(map[int64]db.Word, len(words))
	for _, w := range words {
		wordByID[w.ID] = w
	}

	var meanings []db.WordMeaning
	if err := db.DB.WithContext(ctx).
		Where("word_id IN ? AND kind = ?", wordIDs, "zh").
		Order("word_id, sort_order").Find(&meanings).Error; err != nil {
		log.Printf("[group-detail] find zh meanings failed groupID=%d word_ids=%v err=%v",
			g.ID, wordIDs, err)
		return nil, err
	}
	topByWord := make(map[int64]db.WordMeaning, len(wordIDs))
	for _, mn := range meanings {
		if _, ok := topByWord[mn.WordID]; ok {
			continue
		}
		topByWord[mn.WordID] = mn
	}

	// 保持 word_group_members.id 升序，符合导入的先后顺序
	for _, m := range members {
		w, ok := wordByID[m.WordID]
		if !ok {
			continue
		}
		item := model.SimilarGroupMember{ItemID: w.ID, Word: w.Word}
		if top, ok := topByWord[m.WordID]; ok {
			item.TopMeaningPos = top.PartOfSpeech
			item.TopMeaningText = top.Definition
		}
		data.Members = append(data.Members, item)
	}
	return data, nil
}
