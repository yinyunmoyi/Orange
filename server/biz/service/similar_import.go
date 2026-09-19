package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"aaa_word/biz/converter"
	"aaa_word/biz/db"
	"aaa_word/biz/model"

	"gorm.io/gorm"
)

// EnsureOptions 控制 EnsureWordFullyFavorited 的行为
type EnsureOptions struct {
	// ForceRefresh 为 true 时即使本地已有完整数据也重新联网拉一次
	ForceRefresh bool
}

// EnsureAction 描述一个单词的处理动作
type EnsureAction string

const (
	EnsureFetched       EnsureAction = "fetched"
	EnsureCached        EnsureAction = "cached"
	EnsureLookupMissing EnsureAction = "lookup_missing"
)

// EnsureWordFullyFavorited 保证一个单词拥有辅助学习所需的音频和汉语释义。
// 音标与英英释义尽力从外部词典补充，但词典重试失败或字段缺失不阻断导入。
func EnsureWordFullyFavorited(ctx context.Context, word string, opts EnsureOptions) (int64, EnsureAction, error) {
	if !db.Enabled() {
		return 0, "", db.ErrDBDisabled
	}
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return 0, "", errors.New("importsim: empty word")
	}
	started := time.Now()
	log.Printf("[ensure] start word=%q force_refresh=%t", word, opts.ForceRefresh)

	if !opts.ForceRefresh {
		if id, ok, err := findFullyFavoritedWordID(ctx, word); err != nil {
			log.Printf("[ensure] find fully favorited failed word=%q err=%v", word, err)
			return 0, "", err
		} else if ok {
			log.Printf("[ensure] cache hit word=%q wordID=%d elapsed=%s", word, id, time.Since(started))
			return id, EnsureCached, nil
		} else {
			log.Printf("[ensure] cache miss or incomplete word=%q existing_word_id=%d", word, id)
		}
	} else {
		log.Printf("[ensure] force refresh word=%q", word)
	}

	log.Printf("[ensure] dictionary lookup start word=%q", word)
	wordData, err := LookupWithFallback(word)
	if err != nil {
		log.Printf("[ensure] dictionary unavailable, continue with tts-only data word=%q err=%v",
			word, err)
		wordData = converter.BuildTTSOnlyWordData(word)
	} else {
		log.Printf("[ensure] dictionary lookup ok word=%q phonetic=%q pronunciations=%d english_definitions=%d",
			word, wordData.Phonetic, len(wordData.Pronunciations), englishDefinitionCount(wordData))
	}
	if missing := missingWordDataFields(wordData); len(missing) > 0 {
		fallback := converter.BuildTTSOnlyWordData(word)
		if wordData == nil {
			wordData = fallback
		} else {
			wordData.Word = word
			wordData.Pronunciations = fallback.Pronunciations
		}
		log.Printf("[ensure] dictionary audio missing, use tts fallback word=%q missing=%v", word, missing)
	}

	log.Printf("[ensure] chinese meaning start word=%q", word)
	meaning, err := GetMeaning(word)
	if err != nil {
		log.Printf("[ensure] meaning failed word=%q err=%v", word, err)
		return 0, "", fmt.Errorf("meaning: %w", err)
	}
	if meaning == nil || len(meaning.Meanings) == 0 {
		log.Printf("[ensure] meaning incomplete word=%q", word)
		return 0, "", errors.New("meaning: empty chinese meanings")
	}
	log.Printf("[ensure] chinese meaning ok word=%q meanings=%d", word, len(meaning.Meanings))
	if missing := missingWordDataFields(wordData); len(missing) > 0 {
		log.Printf("[ensure] required data incomplete word=%q missing=%v", word, missing)
		return 0, "", fmt.Errorf("required data incomplete: missing %s", strings.Join(missing, ","))
	}

	req := &model.FavoriteRequest{
		Word:     word,
		WordData: wordData,
		Meaning:  meaning,
	}
	log.Printf("[ensure] persist start word=%q", word)
	wordID, err := AddFavoriteAtLearningQueueTail(ctx, req)
	if err != nil {
		log.Printf("[ensure] add favorite failed word=%q err=%v", word, err)
		return 0, "", fmt.Errorf("add favorite: %w", err)
	}
	log.Printf("[ensure] persist committed word=%q wordID=%d", word, wordID)
	verifiedID, complete, err := findFullyFavoritedWordID(ctx, word)
	if err != nil {
		return 0, "", fmt.Errorf("verify initialized word: %w", err)
	}
	if !complete || verifiedID != wordID {
		log.Printf("[ensure] post-write verification failed word=%q wordID=%d verifiedID=%d complete=%t",
			word, wordID, verifiedID, complete)
		return 0, "", errors.New("verify initialized word: incomplete persisted data")
	}
	log.Printf("[ensure] complete word=%q wordID=%d action=%s elapsed=%s",
		word, wordID, EnsureFetched, time.Since(started))
	return wordID, EnsureFetched, nil
}

func englishDefinitionCount(data *model.WordData) int {
	if data == nil {
		return 0
	}
	count := 0
	for _, meaning := range data.Meanings {
		for _, definition := range meaning.Definitions {
			if strings.TrimSpace(definition.Definition) != "" {
				count++
			}
		}
	}
	return count
}

func missingWordDataFields(data *model.WordData) []string {
	if data == nil {
		return []string{"word_data"}
	}
	missing := make([]string, 0, 1)
	hasAudio := false
	for _, pronunciation := range data.Pronunciations {
		if strings.TrimSpace(pronunciation.AudioURL) != "" {
			hasAudio = true
			break
		}
	}
	if !hasAudio {
		missing = append(missing, "audio")
	}
	return missing
}

type wordInitializationStatus struct {
	Audio          bool
	Phonetic       bool
	EnglishMeaning bool
	ChineseMeaning bool
}

func (status wordInitializationStatus) complete() bool {
	return status.Audio && status.ChineseMeaning
}

func (status wordInitializationStatus) missingFields() []string {
	missing := make([]string, 0, 2)
	if !status.Audio {
		missing = append(missing, "audio")
	}
	if !status.ChineseMeaning {
		missing = append(missing, "chinese_meaning")
	}
	return missing
}

// findFullyFavoritedWordID 判断本地是否已保存音标、音频及中英文释义。
func findFullyFavoritedWordID(ctx context.Context, word string) (int64, bool, error) {
	var w db.Word
	err := db.DB.WithContext(ctx).Where("word = ?", word).Take(&w).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, false, nil
		}
		log.Printf("[ensure-find] take word failed word=%q err=%v", word, err)
		return 0, false, err
	}

	var audios []db.WordAudio
	if err := db.DB.WithContext(ctx).
		Select("phonetic_text", "file_path", "file_size").
		Where("word_id = ?", w.ID).
		Find(&audios).Error; err != nil {
		log.Printf("[ensure-find] find audio failed word=%q wordID=%d err=%v", word, w.ID, err)
		return 0, false, err
	}
	status := wordInitializationStatus{}
	for _, audio := range audios {
		if healthy, _ := favoriteAudioHealthy(audio.FilePath, audio.FileSize); healthy {
			status.Audio = true
		}
		if strings.TrimSpace(audio.PhoneticText) != "" {
			status.Phonetic = true
		}
	}

	var meanings []db.WordMeaning
	if err := db.DB.WithContext(ctx).
		Select("kind", "definition").
		Where("word_id = ? AND kind IN ?", w.ID, []string{"en", "zh"}).
		Find(&meanings).Error; err != nil {
		log.Printf("[ensure-find] find meanings failed word=%q wordID=%d err=%v", word, w.ID, err)
		return 0, false, err
	}
	for _, meaning := range meanings {
		if strings.TrimSpace(meaning.Definition) == "" {
			continue
		}
		switch meaning.Kind {
		case "en":
			status.EnglishMeaning = true
		case "zh":
			status.ChineseMeaning = true
		}
	}
	if !status.complete() {
		log.Printf("[ensure-find] incomplete word=%q wordID=%d missing=%v",
			word, w.ID, status.missingFields())
		return w.ID, false, nil
	}
	return w.ID, true, nil
}

// MergeAction 描述一次 MergeGroup 的动作
type MergeAction string

const (
	MergeCreated MergeAction = "created"
	MergeJoined  MergeAction = "joined"
	MergeMerged  MergeAction = "merged"
	MergeNoop    MergeAction = "noop"
)

// MergeGroup 把一组 word_id 合并到相似单词组中：
// - 全部未归属 → 新建组 (created)
// - 已归属于同一组 → 加入新增成员 (joined) 或无变化 (noop)
// - 归属于多个组 → 以 id 最小的组为主组，其它组成员迁移过来 (merged)
func MergeGroup(ctx context.Context, wordIDs []int64) (int64, MergeAction, error) {
	if !db.Enabled() {
		return 0, "", db.ErrDBDisabled
	}
	// 组内去重
	seen := make(map[int64]struct{}, len(wordIDs))
	uniqueIDs := make([]int64, 0, len(wordIDs))
	for _, id := range wordIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) < 2 {
		return 0, "", errors.New("importsim: merge requires at least 2 word ids")
	}

	var (
		resultGroupID int64
		resultAction  MergeAction
	)
	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existingMembers []db.WordGroupMember
		if err := tx.Where("word_id IN ?", uniqueIDs).Find(&existingMembers).Error; err != nil {
			log.Printf("[merge] query existing members failed wordIDs=%v err=%v", uniqueIDs, err)
			return fmt.Errorf("query existing members: %w", err)
		}
		existingByWord := make(map[int64]int64, len(existingMembers))
		groupSet := make(map[int64]struct{})
		for _, m := range existingMembers {
			existingByWord[m.WordID] = m.GroupID
			groupSet[m.GroupID] = struct{}{}
		}
		existingGroups := make([]int64, 0, len(groupSet))
		for gid := range groupSet {
			existingGroups = append(existingGroups, gid)
		}

		switch len(existingGroups) {
		case 0:
			// 新建组
			g := db.WordGroup{MemberCount: len(uniqueIDs)}
			if err := tx.Create(&g).Error; err != nil {
				log.Printf("[merge] create group failed member_word_ids=%v err=%v", uniqueIDs, err)
				return fmt.Errorf("create group: %w", err)
			}
			rows := make([]db.WordGroupMember, 0, len(uniqueIDs))
			for _, wid := range uniqueIDs {
				rows = append(rows, db.WordGroupMember{GroupID: g.ID, WordID: wid})
			}
			if err := tx.Create(&rows).Error; err != nil {
				log.Printf("[merge] create members failed groupID=%d member_word_ids=%v err=%v", g.ID, uniqueIDs, err)
				return fmt.Errorf("create members: %w", err)
			}
			resultGroupID = g.ID
			resultAction = MergeCreated
			return nil
		case 1:
			// 加入已有组
			mainID := existingGroups[0]
			toAdd := make([]db.WordGroupMember, 0)
			addedIDs := make([]int64, 0)
			for _, wid := range uniqueIDs {
				if _, ok := existingByWord[wid]; ok {
					continue
				}
				toAdd = append(toAdd, db.WordGroupMember{GroupID: mainID, WordID: wid})
				addedIDs = append(addedIDs, wid)
			}
			if len(toAdd) == 0 {
				// 完全无变化：不做任何写入（不触发 updated_at、不 delete/insert）
				resultGroupID = mainID
				resultAction = MergeNoop
				return nil
			}
			if err := tx.Create(&toAdd).Error; err != nil {
				log.Printf("[merge] append members failed groupID=%d added_word_ids=%v err=%v", mainID, addedIDs, err)
				return fmt.Errorf("append members: %w", err)
			}
			if err := recountGroupMembers(tx, mainID); err != nil {
				log.Printf("[merge] recount after join failed groupID=%d err=%v", mainID, err)
				return err
			}
			resultGroupID = mainID
			resultAction = MergeJoined
			return nil
		default:
			// 多组合并
			mainID := existingGroups[0]
			for _, gid := range existingGroups[1:] {
				if gid < mainID {
					mainID = gid
				}
			}
			otherIDs := make([]int64, 0, len(existingGroups)-1)
			for _, gid := range existingGroups {
				if gid != mainID {
					otherIDs = append(otherIDs, gid)
				}
			}
			if err := tx.Model(&db.WordGroupMember{}).
				Where("group_id IN ?", otherIDs).
				Update("group_id", mainID).Error; err != nil {
				log.Printf("[merge] merge members failed mainID=%d others=%v err=%v", mainID, otherIDs, err)
				return fmt.Errorf("merge members: %w", err)
			}
			if err := tx.Where("id IN ?", otherIDs).Delete(&db.WordGroup{}).Error; err != nil {
				log.Printf("[merge] delete merged groups failed others=%v err=%v", otherIDs, err)
				return fmt.Errorf("delete merged groups: %w", err)
			}
			toAdd := make([]db.WordGroupMember, 0)
			addedIDs := make([]int64, 0)
			for _, wid := range uniqueIDs {
				if _, ok := existingByWord[wid]; ok {
					continue
				}
				toAdd = append(toAdd, db.WordGroupMember{GroupID: mainID, WordID: wid})
				addedIDs = append(addedIDs, wid)
			}
			if len(toAdd) > 0 {
				if err := tx.Create(&toAdd).Error; err != nil {
					log.Printf("[merge] append members after merge failed groupID=%d added_word_ids=%v err=%v", mainID, addedIDs, err)
					return fmt.Errorf("append members after merge: %w", err)
				}
			}
			if err := recountGroupMembers(tx, mainID); err != nil {
				log.Printf("[merge] recount after merge failed groupID=%d err=%v", mainID, err)
				return err
			}
			resultGroupID = mainID
			resultAction = MergeMerged
			return nil
		}
	})
	if err != nil {
		log.Printf("[merge] transaction failed wordIDs=%v err=%v", uniqueIDs, err)
		return 0, "", err
	}
	return resultGroupID, resultAction, nil
}

// recountGroupMembers 重算 member_count 并同步 updated_at
func recountGroupMembers(tx *gorm.DB, groupID int64) error {
	var count int64
	if err := tx.Model(&db.WordGroupMember{}).
		Where("group_id = ?", groupID).Count(&count).Error; err != nil {
		log.Printf("[merge-recount] count failed groupID=%d err=%v", groupID, err)
		return fmt.Errorf("recount members: %w", err)
	}
	if err := tx.Model(&db.WordGroup{}).Where("id = ?", groupID).
		Updates(map[string]any{
			"member_count": int(count),
			"updated_at":   time.Now(),
		}).Error; err != nil {
		log.Printf("[merge-recount] update failed groupID=%d count=%d err=%v", groupID, count, err)
		return fmt.Errorf("update member_count: %w", err)
	}
	return nil
}
