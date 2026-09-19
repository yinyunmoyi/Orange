package service

import (
	"context"
	"errors"
	"log"
	"strings"

	"aaa_word/biz/dict"
	"aaa_word/biz/model"
)

// 本地固定数据 —— 语境目前仅内置 creed，命中即返回，否则视为未找到。
// 汉语释义优先走本地 ECDICT，未命中回退到大模型（GetChineseMeaning）。

// contextsStore 单词 -> 所有语境
var contextsStore = map[string][]model.Context{
	"creed": {
		{
			ID:        1,
			Sentence:  "He chose, not the pureblood (which, according to his creed, is the only kind of wizard worth being or knowing), but the half-blood, like himself.",
			Highlight: "creed",
		},
	},
}

// GetMeaning 返回单词的汉语释义。
// 优先查本地 ECDICT；未收录该词或缺少中文释义时，回退到 DeepSeek 实时生成。
func GetMeaning(word string) (*model.MeaningData, error) {
	return GetMeaningContext(context.Background(), word)
}

// GetMeaningContext 是支持取消和超时的 ECDICT 优先释义查询。
func GetMeaningContext(ctx context.Context, word string) (*model.MeaningData, error) {
	if data := lookupChineseFromEcdict(ctx, word); data != nil {
		return data, nil
	}
	return GetChineseMeaningContext(ctx, word)
}

// lookupChineseFromEcdict 从 ECDICT 组装 MeaningData；未命中返回 nil，让调用方回退。
func lookupChineseFromEcdict(ctx context.Context, word string) *model.MeaningData {
	if !dict.Enabled() {
		return nil
	}
	entry, err := dict.Lookup(ctx, word)
	if err != nil {
		if !errors.Is(err, dict.ErrNotFound) {
			log.Printf("[meaning] ecdict lookup err word=%q err=%v", word, err)
		}
		return nil
	}
	meanings := dict.SplitTranslations(entry.Translation)
	if len(meanings) == 0 {
		return nil
	}
	return &model.MeaningData{
		Word:     entry.Word,
		Meanings: meanings,
		Pos:      dict.NormalizePosField(entry.Pos),
		Collins:  entry.Collins,
		Oxford:   entry.Oxford,
		Tags:     dict.SplitTags(entry.Tag),
	}
}

// GetContexts 返回单词的所有语境；未内置则返回 ErrNotFound
func GetContexts(word string) (*model.ContextsData, error) {
	key := strings.ToLower(word)
	contexts, ok := contextsStore[key]
	if !ok {
		return nil, ErrNotFound
	}
	return &model.ContextsData{
		Word:     key,
		Contexts: contexts,
	}, nil
}
