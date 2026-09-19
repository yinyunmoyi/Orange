package model

// ===== 外部 API（api.dictionaryapi.dev）原始结构 =====

// DictEntry 外部 API 返回数组中的单个词条
type DictEntry struct {
	Word      string         `json:"word"`
	Phonetics []DictPhonetic `json:"phonetics"`
	Meanings  []DictMeaning  `json:"meanings"`
}

// DictPhonetic 音标 + 发音音频
type DictPhonetic struct {
	Text  string `json:"text"`
	Audio string `json:"audio"`
}

// DictMeaning 某个词性下的一组释义
type DictMeaning struct {
	PartOfSpeech string           `json:"partOfSpeech"`
	Definitions  []DictDefinition `json:"definitions"`
	Synonyms     []string         `json:"synonyms"`
	Antonyms     []string         `json:"antonyms"`
}

// DictDefinition 单条释义
type DictDefinition struct {
	Definition string   `json:"definition"`
	Example    string   `json:"example"`
	Synonyms   []string `json:"synonyms"`
	Antonyms   []string `json:"antonyms"`
}

// ===== 对前端的响应结构 =====

// LookupRequest POST 请求体
type LookupRequest struct {
	Word string `json:"word"`
}

// BaseResponse 统一响应包裹
type BaseResponse struct {
	Code int       `json:"code"`
	Msg  string    `json:"msg,omitempty"`
	Data *WordData `json:"data,omitempty"`
}

// WordData 返回给前端的单词信息
type WordData struct {
	Word           string          `json:"word"`
	Phonetic       string          `json:"phonetic"`       // 主音标
	Pronunciations []Pronunciation `json:"pronunciations"` // 每个可用口音一项
	Meanings       []Meaning       `json:"meanings"`

	// 词条级元数据（来自 ECDICT，未命中时为零值）
	Pos      string   `json:"pos,omitempty"`      // 词性分布，形如 "n:35/v:20"
	Collins  int      `json:"collins,omitempty"`  // 柯林斯星级 1-5
	Oxford   int      `json:"oxford,omitempty"`   // 牛津核心 3000 标记 0/1
	Tags     []string `json:"tags,omitempty"`     // 拆分后的标签列表，如 ["cet4","toefl"]
	Exchange string   `json:"exchange,omitempty"` // 词形变化，形如 "p:xxx/d:xxx/i:xxx"
}

// Pronunciation 一个可播放的发音（对应前端一个发音按钮）
type Pronunciation struct {
	Accent   string `json:"accent"`   // UK / US / AU / EN
	Text     string `json:"text"`     // 该发音对应的音标（可能为空）
	AudioURL string `json:"audioUrl"` // 指向本服务音频代理接口的地址
}

// Meaning 某个词性下的释义组
type Meaning struct {
	PartOfSpeech string       `json:"partOfSpeech"`
	Definitions  []Definition `json:"definitions"`
}

// Definition 单条释义
type Definition struct {
	Definition string   `json:"definition"`
	Example    string   `json:"example,omitempty"`
	Synonyms   []string `json:"synonyms"`
	Antonyms   []string `json:"antonyms"`
}

// ===== 汉语释义接口 =====

// MeaningResponse 汉语释义接口的响应包裹
type MeaningResponse struct {
	Code int          `json:"code"`
	Msg  string       `json:"msg,omitempty"`
	Data *MeaningData `json:"data,omitempty"`
}

// MeaningData 单词的汉语释义（可能有多个义项，按使用频率由高到低排序）
type MeaningData struct {
	Word     string           `json:"word"`
	Meanings []ChineseMeaning `json:"meanings"`

	// 词条级元数据（来自 ECDICT，未命中时为零值）
	Pos     string   `json:"pos,omitempty"`
	Collins int      `json:"collins,omitempty"`
	Oxford  int      `json:"oxford,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

// ChineseMeaning 单条汉语释义
type ChineseMeaning struct {
	PartOfSpeech string `json:"partOfSpeech"` // 词性，如 n. / v. / adj.（可能为空）
	Meaning      string `json:"meaning"`      // 简洁的汉语释义
}

// ===== 语境接口 =====

// ContextsResponse 语境接口的响应包裹
type ContextsResponse struct {
	Code int           `json:"code"`
	Msg  string        `json:"msg,omitempty"`
	Data *ContextsData `json:"data,omitempty"`
}

// ContextsData 单词的所有语境
type ContextsData struct {
	Word     string    `json:"word"`
	Contexts []Context `json:"contexts"`
}

// Context 遇到该词的单个语境
type Context struct {
	ID             int64              `json:"id"`
	ItemType       string             `json:"itemType,omitempty"`
	ItemID         int64              `json:"itemId,omitempty"`
	Sentence       string             `json:"sentence"`
	Translation    string             `json:"translation,omitempty"`
	Highlight      string             `json:"highlight"` // 兼容字段：句中需高亮的词
	HighlightStart int                `json:"highlightStart"`
	HighlightEnd   int                `json:"highlightEnd"`
	AudioURL       string             `json:"audioUrl,omitempty"`
	ContextType    string             `json:"contextType,omitempty"`
	Videos         []ContextVideoData `json:"videos,omitempty"`
}

// ===== AI 语境解释接口 =====

// ExplainRequest 语境解释请求体
type ExplainRequest struct {
	Word      string `json:"word"`
	Context   string `json:"context"`
	WordStart int    `json:"wordStart"`
	WordEnd   int    `json:"wordEnd"`
}

// ExplainData 单词在具体语境中的一句自然汉语解释
type ExplainData struct {
	Word        string `json:"word"`
	Explanation string `json:"explanation"`
}

// ExplainResponse 语境解释响应
type ExplainResponse struct {
	Code int          `json:"code"`
	Msg  string       `json:"msg,omitempty"`
	Data *ExplainData `json:"data,omitempty"`
}

// ===== 短语汉语释义接口 =====

// PhraseRequest 短语汉语释义请求体
type PhraseRequest struct {
	Phrase string `json:"phrase"`
}

// ===== 收藏接口 =====

// FavoriteRequest 前端将当前页面已加载的三段数据整体上送
type FavoriteRequest struct {
	Word     string       `json:"word"`
	WordData *WordData    `json:"wordData"`
	Meaning  *MeaningData `json:"meaning"`
}

// FavoriteResponse 收藏结果
type FavoriteResponse struct {
	Code int           `json:"code"`
	Msg  string        `json:"msg,omitempty"`
	Data *FavoriteData `json:"data,omitempty"`
}

// FavoriteData 收藏成功后返回收藏项 ID，供后续语境操作使用。
type FavoriteData struct {
	ItemID int64 `json:"itemId"`
	Level  int   `json:"level"`
}

type FavoriteDeleteResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg,omitempty"`
}

type FavoriteResetResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg,omitempty"`
}

// FavoriteStatusData 收藏状态数据
type FavoriteStatusData struct {
	Favorited bool  `json:"favorited"`
	ItemID    int64 `json:"itemId,omitempty"`
	Level     int   `json:"level,omitempty"`
}

// FavoriteStatusResponse 收藏状态响应
type FavoriteStatusResponse struct {
	Code int                 `json:"code"`
	Msg  string              `json:"msg,omitempty"`
	Data *FavoriteStatusData `json:"data,omitempty"`
}

// FavoriteListItem 收藏列表页展示用的单个卡片摘要
type FavoriteListItem struct {
	ItemType       string `json:"itemType"`
	ItemID         int64  `json:"itemId"`
	Text           string `json:"text"`
	Word           string `json:"word,omitempty"`
	Level          int    `json:"level,omitempty"`
	CreatedAt      string `json:"createdAt"` // RFC3339，前端自行本地化
	TopMeaningPOS  string `json:"topMeaningPos,omitempty"`
	TopMeaningText string `json:"topMeaningText,omitempty"`
}

// FavoriteListData 列表响应数据
type FavoriteListData struct {
	Items []FavoriteListItem `json:"items"`
	Total int64              `json:"total"`
}

type FavoriteGroupSummary struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	RangeStart   string `json:"rangeStart"`
	RangeEnd     string `json:"rangeEnd"`
	DisplayStart string `json:"displayStart"`
	DisplayEnd   string `json:"displayEnd"`
	Count        int64  `json:"count"`
}

type FavoriteGroupListData struct {
	Groups []FavoriteGroupSummary `json:"groups"`
	Total  int64                  `json:"total"`
}

type FavoriteGroupListResponse struct {
	Code int                    `json:"code"`
	Msg  string                 `json:"msg,omitempty"`
	Data *FavoriteGroupListData `json:"data,omitempty"`
}

type FavoritePageData struct {
	Items      []FavoriteListItem `json:"items"`
	NextCursor string             `json:"nextCursor,omitempty"`
}

type FavoritePageResponse struct {
	Code int               `json:"code"`
	Msg  string            `json:"msg,omitempty"`
	Data *FavoritePageData `json:"data,omitempty"`
}

// FavoriteListResponse 列表接口的响应包裹
type FavoriteListResponse struct {
	Code int               `json:"code"`
	Msg  string            `json:"msg,omitempty"`
	Data *FavoriteListData `json:"data,omitempty"`
}

// ===== 相似单词组接口 =====

// SimilarGroupSummary 相似单词组列表卡片摘要
type SimilarGroupSummary struct {
	ID          int64    `json:"id"`
	MemberCount int      `json:"memberCount"`
	Preview     []string `json:"preview"`             // 组内前若干个单词，用于卡片预览
	UpdatedAt   string   `json:"updatedAt,omitempty"` // RFC3339
}

// SimilarGroupListData 列表响应数据
type SimilarGroupListData struct {
	Items []SimilarGroupSummary `json:"items"`
	Total int64                 `json:"total"`
}

// SimilarGroupListResponse 列表响应包裹
type SimilarGroupListResponse struct {
	Code int                   `json:"code"`
	Msg  string                `json:"msg,omitempty"`
	Data *SimilarGroupListData `json:"data,omitempty"`
}

// SimilarGroupMember 详情页单个成员卡片
type SimilarGroupMember struct {
	ItemID         int64  `json:"itemId"`
	Word           string `json:"word"`
	TopMeaningPos  string `json:"topMeaningPos,omitempty"`
	TopMeaningText string `json:"topMeaningText,omitempty"`
}

// SimilarGroupDetailData 详情响应数据
type SimilarGroupDetailData struct {
	ID          int64                `json:"id"`
	MemberCount int                  `json:"memberCount"`
	Members     []SimilarGroupMember `json:"members"`
	UpdatedAt   string               `json:"updatedAt,omitempty"`
}

// SimilarGroupDetailResponse 详情响应包裹
type SimilarGroupDetailResponse struct {
	Code int                     `json:"code"`
	Msg  string                  `json:"msg,omitempty"`
	Data *SimilarGroupDetailData `json:"data,omitempty"`
}

type SimilarGroupMemberAddRequest struct {
	Word          string `json:"word"`
	CandidateWord string `json:"candidateWord"`
}

// ===== 易混词联想接口 =====

type WordAssociationRequest struct {
	Word         string `json:"word"`
	MeaningHint  string `json:"meaningHint"`
	SpellingHint string `json:"spellingHint"`
}

type WordAssociationItem struct {
	Word           string  `json:"word"`
	TopMeaningText string  `json:"topMeaningText"`
	Similarity     float64 `json:"similarity"`
}

type WordAssociationData struct {
	Items []WordAssociationItem `json:"items"`
}

type WordAssociationResponse struct {
	Code int                  `json:"code"`
	Msg  string               `json:"msg,omitempty"`
	Data *WordAssociationData `json:"data,omitempty"`
}
