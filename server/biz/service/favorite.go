package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"time"

	"aaa_word/biz/converter"
	"aaa_word/biz/db"
	"aaa_word/biz/dict"
	"aaa_word/biz/model"
	"aaa_word/biz/storage"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AddFavorite 收藏一个单词：把前端传来的词典数据一次性落到 DB，并同步下载所有音频到本地。
// 事务内先写元数据，再逐个下载音频；任一步失败则回滚事务 + 清理已落盘的临时文件。
func AddFavorite(ctx context.Context, req *model.FavoriteRequest) (int64, int, error) {
	return addFavorite(ctx, req, false)
}

// AddFavoriteAtLearningQueueTail 用于导入辅助学习词：收藏数据照常初始化，
// 但新建的学习项排在现有未学习项之后，不抢占用户主动收藏词的优先级。
func AddFavoriteAtLearningQueueTail(ctx context.Context, req *model.FavoriteRequest) (int64, error) {
	itemID, _, err := addFavorite(ctx, req, true)
	return itemID, err
}

func addFavorite(ctx context.Context, req *model.FavoriteRequest, learningQueueTail bool) (int64, int, error) {
	if !db.Enabled() {
		log.Printf("[favorite] db disabled")
		return 0, 0, db.ErrDBDisabled
	}
	if req == nil || req.WordData == nil {
		log.Printf("[favorite] missing word data req_nil=%t", req == nil)
		return 0, 0, errors.New("favorite: missing word data")
	}
	wordName := strings.ToLower(strings.TrimSpace(req.Word))
	if wordName == "" {
		log.Printf("[favorite] empty word raw=%q", req.Word)
		return 0, 0, errors.New("favorite: empty word")
	}

	var savedPaths []string
	var wordID int64
	var wordLevel int

	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. upsert words
		wordRow := db.Word{Word: wordName, Level: 1}
		createWord := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "word"}},
			DoNothing: true,
		}).Create(&wordRow)
		if createWord.Error != nil {
			log.Printf("[favorite] upsert word failed word=%q err=%v", wordName, createWord.Error)
			return fmt.Errorf("upsert word: %w", createWord.Error)
		}
		wordCreated := createWord.RowsAffected == 1
		// OnConflict 时 ID 可能未回填，兜底重查一次
		if wordRow.ID == 0 {
			if err := tx.Where("word = ?", wordName).Take(&wordRow).Error; err != nil {
				log.Printf("[favorite] fetch word after upsert failed word=%q err=%v", wordName, err)
				return fmt.Errorf("fetch word after upsert: %w", err)
			}
		}
		wordID = wordRow.ID
		wordLevel = wordRow.Level
		if wordLevel < 1 {
			wordLevel = 1
			if err := tx.Model(&db.Word{}).Where("id = ?", wordID).Update("level", wordLevel).Error; err != nil {
				return fmt.Errorf("normalize word level: %w", err)
			}
		}

		queuedAt := time.Now()
		if learningQueueTail {
			var err error
			queuedAt, err = learningQueueTailTime(tx, queuedAt)
			if err != nil {
				log.Printf("[favorite] resolve learning queue tail failed word=%q wordID=%d err=%v",
					wordName, wordID, err)
				return fmt.Errorf("resolve learning queue tail: %w", err)
			}
		}
		if err := initializeItemLearning(tx, LearningItemWord, wordID, queuedAt); err != nil {
			log.Printf("[favorite] initialize learning failed word=%q wordID=%d err=%v", wordName, wordID, err)
			return fmt.Errorf("initialize learning: %w", err)
		}
		log.Printf("[favorite] learning initialized word=%q wordID=%d queue_tail=%t queued_at=%s",
			wordName, wordID, learningQueueTail, queuedAt.Format(time.RFC3339Nano))

		// 2. 清空旧数据（收藏即覆盖，允许重复收藏刷新内容）
		if err := tx.Where("word_id = ?", wordID).Delete(&db.WordMeaning{}).Error; err != nil {
			log.Printf("[favorite] clean meanings failed word=%q wordID=%d err=%v", wordName, wordID, err)
			return fmt.Errorf("clean meanings: %w", err)
		}
		if err := tx.Where("word_id = ?", wordID).Delete(&db.WordAudio{}).Error; err != nil {
			log.Printf("[favorite] clean audios failed word=%q wordID=%d err=%v", wordName, wordID, err)
			return fmt.Errorf("clean audios: %w", err)
		}

		// 3. 英英释义
		if len(req.WordData.Meanings) > 0 {
			enRows := make([]db.WordMeaning, 0, len(req.WordData.Meanings)*2)
			enPOS := make([]string, 0, len(req.WordData.Meanings))
			order := 0
			for _, m := range req.WordData.Meanings {
				enPOS = append(enPOS, m.PartOfSpeech)
				for _, d := range m.Definitions {
					enRows = append(enRows, db.WordMeaning{
						WordID:       wordID,
						Kind:         "en",
						PartOfSpeech: m.PartOfSpeech,
						Definition:   d.Definition,
						Example:      d.Example,
						Synonyms:     db.JSONStringSlice(d.Synonyms),
						Antonyms:     db.JSONStringSlice(d.Antonyms),
						SortOrder:    order,
					})
					order++
				}
			}
			if len(enRows) > 0 {
				if err := tx.Create(&enRows).Error; err != nil {
					log.Printf("[favorite] insert en meanings failed word=%q wordID=%d pos=%v defs=%d err=%v",
						wordName, wordID, enPOS, len(enRows), err)
					return fmt.Errorf("insert en meanings: %w", err)
				}
			}
		}

		// 4. 汉语释义
		if req.Meaning != nil && len(req.Meaning.Meanings) > 0 {
			zhRows := make([]db.WordMeaning, 0, len(req.Meaning.Meanings))
			zhPOS := make([]string, 0, len(req.Meaning.Meanings))
			for i, cm := range req.Meaning.Meanings {
				zhPOS = append(zhPOS, cm.PartOfSpeech)
				zhRows = append(zhRows, db.WordMeaning{
					WordID:       wordID,
					Kind:         "zh",
					PartOfSpeech: cm.PartOfSpeech,
					Definition:   cm.Meaning,
					SortOrder:    i,
				})
			}
			if err := tx.Create(&zhRows).Error; err != nil {
				log.Printf("[favorite] insert zh meanings failed word=%q wordID=%d pos=%v err=%v",
					wordName, wordID, zhPOS, err)
				return fmt.Errorf("insert zh meanings: %w", err)
			}
		}

		// 5. 同步下载音频并落盘。以 accent 去重（同一口音只保留一条，覆盖后来的）
		seenAccent := make(map[string]struct{})
		for _, pron := range req.WordData.Pronunciations {
			if _, dup := seenAccent[pron.Accent]; dup {
				log.Printf("[favorite] skip duplicate accent word=%q accent=%q", wordName, pron.Accent)
				continue
			}
			sourceURL := extractSourceURL(pron.AudioURL)
			if sourceURL == "" {
				log.Printf("[favorite] skip empty audio src word=%q accent=%q raw_audio_url=%q",
					wordName, pron.Accent, pron.AudioURL)
				continue
			}
			if err := ValidateAudioSrc(sourceURL); err != nil {
				log.Printf("[favorite] invalid audio src word=%q accent=%q src=%q err=%v",
					wordName, pron.Accent, sourceURL, err)
				return fmt.Errorf("invalid audio src %q: %w", sourceURL, err)
			}

			var (
				filePath    string
				fileSize    int64
				contentType string
			)
			if ttsWord, ttsAccent, isTTS := ParseTTSSrc(sourceURL); isTTS {
				mp3Bytes, ct, err := SynthesizeMP3(ttsWord, ttsAccent)
				if err != nil {
					log.Printf("[favorite] tts synth failed word=%q accent=%q src=%q err=%v",
						wordName, pron.Accent, sourceURL, err)
					return fmt.Errorf("synthesize tts audio %q: %w", sourceURL, err)
				}
				path, size, saveErr := storage.SaveFromReader(wordName, pron.Accent, bytes.NewReader(mp3Bytes))
				if saveErr != nil {
					log.Printf("[favorite] save tts audio failed word=%q accent=%q src=%q err=%v",
						wordName, pron.Accent, sourceURL, saveErr)
					return fmt.Errorf("save tts audio: %w", saveErr)
				}
				filePath, fileSize, contentType = path, size, ct
				log.Printf("[favorite] tts audio saved word=%q accent=%q src=%q path=%s size=%d ct=%s",
					wordName, pron.Accent, sourceURL, filePath, fileSize, contentType)
			} else {
				body, ct, err := StreamAudio(sourceURL)
				if err != nil {
					log.Printf("[favorite] download audio failed word=%q accent=%q src=%q err=%v",
						wordName, pron.Accent, sourceURL, err)
					return fmt.Errorf("download audio %q: %w", sourceURL, err)
				}
				path, size, saveErr := storage.SaveFromReader(wordName, pron.Accent, body)
				body.Close()
				if saveErr != nil {
					log.Printf("[favorite] save audio failed word=%q accent=%q src=%q err=%v",
						wordName, pron.Accent, sourceURL, saveErr)
					return fmt.Errorf("save audio: %w", saveErr)
				}
				filePath, fileSize, contentType = path, size, ct
				log.Printf("[favorite] audio saved word=%q accent=%q src=%q path=%s size=%d ct=%s",
					wordName, pron.Accent, sourceURL, filePath, fileSize, contentType)
			}
			savedPaths = append(savedPaths, filePath)

			row := db.WordAudio{
				WordID:       wordID,
				Accent:       pron.Accent,
				PhoneticText: pron.Text,
				SourceURL:    sourceURL,
				FilePath:     filePath,
				ContentType:  contentType,
				FileSize:     fileSize,
			}
			if err := tx.Create(&row).Error; err != nil {
				log.Printf("[favorite] insert audio failed word=%q accent=%q src=%q err=%v",
					wordName, pron.Accent, sourceURL, err)
				return fmt.Errorf("insert audio: %w", err)
			}
			seenAccent[pron.Accent] = struct{}{}
		}
		if wordCreated {
			level := wordLevel
			recordActionBestEffort(tx, actionEventParams{
				EventID:  serverActionEventID(model.ActionFavoriteCreated, ContextItemWord, wordID),
				ItemType: ContextItemWord, ItemID: wordID, ItemText: wordName,
				Action: model.ActionFavoriteCreated, Source: model.ActionSourceSystem,
				LevelBefore: &level, LevelAfter: &level, OccurredAt: time.Now(),
			})
		}
		return nil
	})

	if err != nil {
		log.Printf("[favorite] transaction rollback word=%q saved_paths=%v err=%v",
			wordName, savedPaths, err)
		// 事务回滚了，需要把已落盘的音频文件也清掉
		for _, p := range savedPaths {
			storage.Delete(p)
		}
		return 0, 0, err
	}
	log.Printf("[favorite] committed word=%q word_id=%d saved_audio_paths=%v", wordName, wordID, savedPaths)
	return wordID, wordLevel, nil
}

func learningQueueTailTime(tx *gorm.DB, fallback time.Time) (time.Time, error) {
	var result struct {
		Oldest *time.Time `gorm:"column:oldest"`
	}
	if err := tx.Model(&db.UserItemLearning{}).
		Select("MIN(queued_at) AS oldest").
		Scan(&result).Error; err != nil {
		return time.Time{}, err
	}
	if result.Oldest == nil {
		return fallback, nil
	}
	return result.Oldest.Add(-time.Microsecond), nil
}

// IsFavorited 判断该词是否已被收藏，若已收藏则一并返回其 ID（用于后续语境操作）。
func IsFavorited(ctx context.Context, word string) (bool, int64, int, error) {
	if !db.Enabled() {
		return false, 0, 0, db.ErrDBDisabled
	}
	var row db.Word
	if err := db.DB.WithContext(ctx).Select("id", "level").
		Where("word = ?", strings.ToLower(word)).
		Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, 0, 0, nil
		}
		log.Printf("[favorite-status] lookup failed word=%q err=%v", word, err)
		return false, 0, 0, err
	}
	return true, row.ID, row.Level, nil
}

// ListFavorites 列出已收藏单词和短语，并附带第一条中文释义。
// query 非空时按前缀匹配（依赖 uk_word / uk_phrase B-tree 唯一索引）。
func ListFavorites(ctx context.Context, query string) (*model.FavoriteListData, error) {
	if !db.Enabled() {
		return nil, db.ErrDBDisabled
	}
	q := strings.ToLower(strings.TrimSpace(query))
	wordQuery := db.DB.WithContext(ctx).Model(&db.Word{})
	phraseQuery := db.DB.WithContext(ctx).Model(&db.Phrase{})
	if q != "" {
		wordQuery = wordQuery.Where("word LIKE ?", q+"%")
		phraseQuery = phraseQuery.Where("phrase LIKE ?", q+"%")
	}
	var words []db.Word
	if err := wordQuery.Order("created_at DESC").Find(&words).Error; err != nil {
		log.Printf("[favorites] find words failed query=%q err=%v", query, err)
		return nil, err
	}
	var phrases []db.Phrase
	if err := phraseQuery.Order("created_at DESC").Find(&phrases).Error; err != nil {
		log.Printf("[favorites] find phrases failed query=%q err=%v", query, err)
		return nil, err
	}

	wordIDs := make([]int64, 0, len(words))
	wordSamples := make([]string, 0, len(words))
	for _, w := range words {
		wordIDs = append(wordIDs, w.ID)
		wordSamples = append(wordSamples, w.Word)
	}
	var wordMeanings []db.WordMeaning
	if len(wordIDs) > 0 {
		if err := db.DB.WithContext(ctx).
			Where("word_id IN ? AND kind = ?", wordIDs, "zh").
			Order("word_id, sort_order").Find(&wordMeanings).Error; err != nil {
			log.Printf("[favorites] find word meanings failed query=%q word_ids=%v words=%v err=%v",
				query, wordIDs, wordSamples, err)
			return nil, err
		}
	}
	topByWord := make(map[int64]db.WordMeaning, len(wordIDs))
	for _, m := range wordMeanings {
		if _, ok := topByWord[m.WordID]; ok {
			continue
		}
		topByWord[m.WordID] = m
	}

	phraseIDs := make([]int64, 0, len(phrases))
	for _, phrase := range phrases {
		phraseIDs = append(phraseIDs, phrase.ID)
	}
	var phraseMeanings []db.PhraseMeaning
	if len(phraseIDs) > 0 {
		if err := db.DB.WithContext(ctx).Where("phrase_id IN ?", phraseIDs).
			Order("phrase_id, sort_order").Find(&phraseMeanings).Error; err != nil {
			log.Printf("[favorites] find phrase meanings failed query=%q phrase_ids=%v err=%v",
				query, phraseIDs, err)
			return nil, err
		}
	}
	topByPhrase := make(map[int64]db.PhraseMeaning, len(phraseIDs))
	for _, meaning := range phraseMeanings {
		if _, ok := topByPhrase[meaning.PhraseID]; ok {
			continue
		}
		topByPhrase[meaning.PhraseID] = meaning
	}

	data := &model.FavoriteListData{
		Items: make([]model.FavoriteListItem, 0, len(words)+len(phrases)),
		Total: int64(len(words) + len(phrases)),
	}
	for _, w := range words {
		item := model.FavoriteListItem{
			ItemType:  ContextItemWord,
			ItemID:    w.ID,
			Text:      w.Word,
			Word:      w.Word,
			Level:     w.Level,
			CreatedAt: w.CreatedAt.Format(time.RFC3339),
		}
		if top, ok := topByWord[w.ID]; ok {
			item.TopMeaningPOS = top.PartOfSpeech
			item.TopMeaningText = top.Definition
		}
		data.Items = append(data.Items, item)
	}
	for _, phrase := range phrases {
		item := model.FavoriteListItem{
			ItemType:  ContextItemPhrase,
			ItemID:    phrase.ID,
			Text:      phrase.Phrase,
			CreatedAt: phrase.CreatedAt.Format(time.RFC3339),
		}
		if top, ok := topByPhrase[phrase.ID]; ok {
			item.TopMeaningPOS = top.PartOfSpeech
			item.TopMeaningText = top.Definition
		}
		data.Items = append(data.Items, item)
	}
	sort.SliceStable(data.Items, func(i, j int) bool {
		return data.Items[i].CreatedAt > data.Items[j].CreatedAt
	})
	return data, nil
}

// LoadWordDataFromDB 从数据库拉出 WordData（对应 /lookup 接口的返回）
// 未命中返回 ErrNotFound
func LoadWordDataFromDB(ctx context.Context, word string) (*model.WordData, error) {
	if !db.Enabled() {
		return nil, ErrNotFound
	}
	name := strings.ToLower(word)

	var w db.Word
	if err := db.DB.WithContext(ctx).Where("word = ?", name).Take(&w).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		log.Printf("[favorite-load] take word failed word=%q err=%v", name, err)
		return nil, err
	}

	var audios []db.WordAudio
	if err := db.DB.WithContext(ctx).Where("word_id = ?", w.ID).Order("id").Find(&audios).Error; err != nil {
		log.Printf("[favorite-load] find audios failed word=%q wordID=%d err=%v", name, w.ID, err)
		return nil, err
	}

	var meanings []db.WordMeaning
	if err := db.DB.WithContext(ctx).
		Where("word_id = ? AND kind = ?", w.ID, "en").
		Order("sort_order").Find(&meanings).Error; err != nil {
		log.Printf("[favorite-load] find en meanings failed word=%q wordID=%d err=%v", name, w.ID, err)
		return nil, err
	}

	data := &model.WordData{Word: w.Word}
	// 主音标：第一个非空 phonetic_text
	for _, a := range audios {
		if strings.TrimSpace(a.PhoneticText) != "" {
			data.Phonetic = a.PhoneticText
			break
		}
	}
	// pronunciations：AudioURL 依旧走 /audio?src=<原外链>，命中本地由 handler 自动切换到 file
	for _, a := range audios {
		data.Pronunciations = append(data.Pronunciations, model.Pronunciation{
			Accent:   a.Accent,
			Text:     a.PhoneticText,
			AudioURL: converter.AudioProxyPath + "?src=" + url.QueryEscape(a.SourceURL),
		})
	}
	// meanings：按 part_of_speech 分组，保留 sort_order 顺序
	data.Meanings = groupEnMeanings(meanings)
	return data, nil
}

// LoadMeaningFromDB 从数据库拉出汉语释义
func LoadMeaningFromDB(ctx context.Context, word string) (*model.MeaningData, error) {
	if !db.Enabled() {
		return nil, ErrNotFound
	}
	name := strings.ToLower(word)

	var w db.Word
	if err := db.DB.WithContext(ctx).Where("word = ?", name).Take(&w).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		log.Printf("[favorite-load-meaning] take word failed word=%q err=%v", name, err)
		return nil, err
	}

	var rows []db.WordMeaning
	if err := db.DB.WithContext(ctx).
		Where("word_id = ? AND kind = ?", w.ID, "zh").
		Order("sort_order").Find(&rows).Error; err != nil {
		log.Printf("[favorite-load-meaning] find zh meanings failed word=%q wordID=%d err=%v", name, w.ID, err)
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	data := &model.MeaningData{Word: name}
	for _, r := range rows {
		data.Meanings = append(data.Meanings, model.ChineseMeaning{
			PartOfSpeech: r.PartOfSpeech,
			Meaning:      r.Definition,
		})
	}
	return data, nil
}

// LoadContextsFromDB 从数据库拉出语境
func LoadContextsFromDB(ctx context.Context, word string) (*model.ContextsData, error) {
	if !db.Enabled() {
		return nil, ErrNotFound
	}
	name := strings.ToLower(strings.TrimSpace(word))
	var row db.Word
	if err := db.DB.WithContext(ctx).Where("word = ?", name).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		log.Printf("[favorite-load-contexts] resolve word failed word=%q err=%v", name, err)
		return nil, err
	}
	return ListContexts(ctx, ContextItemWord, row.ID)
}

// LookupLocalAudioPath 根据外链 source_url 查本地音频；未命中或 DB 未启用返回 ok=false
func LookupLocalAudioPath(ctx context.Context, sourceURL string) (path, contentType string, ok bool, err error) {
	if !db.Enabled() {
		return "", "", false, nil
	}
	var rows []db.WordAudio
	// 使用 Find + Limit(1) 而非 Take，避免 record-not-found 被 GORM 记为 warning 刷屏日志
	if err = db.DB.WithContext(ctx).Where("source_url = ?", sourceURL).Limit(1).Find(&rows).Error; err != nil {
		log.Printf("[audio-local] find local audio failed src=%q err=%v", sourceURL, err)
		return "", "", false, err
	}
	if len(rows) == 0 {
		return "", "", false, nil
	}
	return rows[0].FilePath, rows[0].ContentType, true, nil
}

// extractSourceURL 从形如 /api/v1/word/audio?src=<encoded> 的 AudioURL 中还原原始外链
func extractSourceURL(audioURL string) string {
	if audioURL == "" {
		return ""
	}
	u, err := url.Parse(audioURL)
	if err != nil {
		return ""
	}
	src := u.Query().Get("src")
	// 兼容前端可能直接传外链的情况
	if src == "" && strings.HasPrefix(audioURL, "http") {
		return audioURL
	}
	return src
}

// groupEnMeanings 按 part_of_speech 聚合相邻的英英释义
func groupEnMeanings(rows []db.WordMeaning) []model.Meaning {
	if len(rows) == 0 {
		return nil
	}
	result := make([]model.Meaning, 0)
	idxByPOS := make(map[string]int)
	for _, r := range rows {
		partOfSpeech, definition := dict.NormalizeDefinition(r.PartOfSpeech, r.Definition)
		def := model.Definition{
			Definition: definition,
			Example:    r.Example,
			Synonyms:   []string(r.Synonyms),
			Antonyms:   []string(r.Antonyms),
		}
		if idx, ok := idxByPOS[partOfSpeech]; ok {
			result[idx].Definitions = append(result[idx].Definitions, def)
			continue
		}
		result = append(result, model.Meaning{
			PartOfSpeech: partOfSpeech,
			Definitions:  []model.Definition{def},
		})
		idxByPOS[partOfSpeech] = len(result) - 1
	}
	return result
}
