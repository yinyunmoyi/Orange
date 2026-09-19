package converter

import (
	"net/url"
	"strings"

	"aaa_word/biz/model"
)

// AudioProxyPath 音频代理接口路径；前端音频地址在此基础上拼接 ?src=
const AudioProxyPath = "/api/v1/word/audio"

// ttsUSSrcPrefix 词典 API 无美音音频时，使用该内部特殊标识作为 audio src；
// 与 service.BuildTTSSrc 保持一致，抽在这里避免 converter 反向依赖 service 造成循环
const ttsUSSrcPrefix = "tts://us/"

// ToWordData 将外部 API 返回的所有词条合并、显式映射为返回前端的结构。
// 词典 API 对同一个词可能返回多个词条（如 flounder：一个 noun 词条 + 一个 verb 词条），
// 这里会把各词条的释义依次拼接。发音统一走火山 TTS，只输出一条美音。
func ToWordData(entries []model.DictEntry) *model.WordData {
	data := &model.WordData{}
	if len(entries) == 0 {
		return data
	}

	data.Word = entries[0].Word

	// 汇总所有词条的音标与释义
	var allPhonetics []model.DictPhonetic
	var allMeanings []model.DictMeaning
	for i := range entries {
		allPhonetics = append(allPhonetics, entries[i].Phonetics...)
		allMeanings = append(allMeanings, entries[i].Meanings...)
	}

	data.Phonetic = pickMainPhonetic(allPhonetics)
	data.Meanings = toMeanings(allMeanings)
	// 发音只保留一条：走火山 TTS 合成的美音，忽略词典自带的音频链接（CDN 不稳定）
	data.Pronunciations = buildTTSPronunciations(data.Word, data.Phonetic)
	return data
}

// buildTTSPronunciations 生成仅含 US TTS 发音的列表；word 空则返回空列表
func buildTTSPronunciations(word, phonetic string) []model.Pronunciation {
	w := strings.TrimSpace(word)
	if w == "" {
		return nil
	}
	src := ttsUSSrcPrefix + strings.ToLower(w)
	return []model.Pronunciation{{
		Accent:   "US",
		Text:     phonetic,
		AudioURL: AudioProxyPath + "?src=" + url.QueryEscape(src),
	}}
}

// BuildUSPronunciations 对外暴露 US TTS 发音列表的构造能力，供 ECDICT 等本地数据源复用。
func BuildUSPronunciations(word, phonetic string) []model.Pronunciation {
	return buildTTSPronunciations(word, phonetic)
}

// BuildTTSOnlyWordData 构造一个只含单词名 + US TTS 兜底发音的最小 WordData，
// 用于外部词典 API 返回 404（如 Hogwarts 这种专有名词）时，仍向前端提供发音按钮
func BuildTTSOnlyWordData(word string) *model.WordData {
	w := strings.TrimSpace(word)
	if w == "" {
		return &model.WordData{}
	}
	return &model.WordData{
		Word:           w,
		Pronunciations: buildTTSPronunciations(w, ""),
	}
}

// pickMainPhonetic 取第一个非空 text 作为主音标
func pickMainPhonetic(phonetics []model.DictPhonetic) string {
	for _, p := range phonetics {
		if strings.TrimSpace(p.Text) != "" {
			return p.Text
		}
	}
	return ""
}

// toMeanings 显式映射释义组
func toMeanings(meanings []model.DictMeaning) []model.Meaning {
	result := make([]model.Meaning, 0, len(meanings))
	for _, m := range meanings {
		defs := make([]model.Definition, 0, len(m.Definitions))
		for _, d := range m.Definitions {
			defs = append(defs, model.Definition{
				Definition: d.Definition,
				Example:    d.Example,
				Synonyms:   d.Synonyms,
				Antonyms:   d.Antonyms,
			})
		}
		result = append(result, model.Meaning{
			PartOfSpeech: m.PartOfSpeech,
			Definitions:  defs,
		})
	}
	return result
}
