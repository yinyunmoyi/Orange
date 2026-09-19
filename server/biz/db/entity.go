package db

import (
	"time"

	"gorm.io/gorm"
)

// Word 收藏词主表
type Word struct {
	ID        int64     `gorm:"primaryKey;column:id;index:idx_words_created_at,priority:2"`
	Word      string    `gorm:"column:word;size:64;uniqueIndex:uk_word;not null"`
	Level     int       `gorm:"column:level;not null;default:1"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime;index:idx_words_created_at,priority:1"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (Word) TableName() string { return "words" }

// WordMeaning 词性 + 释义，中英统一表；由 Kind 区分
type WordMeaning struct {
	ID           int64           `gorm:"primaryKey;column:id"`
	WordID       int64           `gorm:"column:word_id;not null;index:idx_word_id_kind,priority:1"`
	Kind         string          `gorm:"column:kind;type:enum('zh','en');not null;index:idx_word_id_kind,priority:2"`
	PartOfSpeech string          `gorm:"column:part_of_speech;size:32;not null;default:''"`
	Definition   string          `gorm:"column:definition;type:text;not null"`
	Example      string          `gorm:"column:example;type:text;not null"`
	Synonyms     JSONStringSlice `gorm:"column:synonyms;type:json"`
	Antonyms     JSONStringSlice `gorm:"column:antonyms;type:json"`
	SortOrder    int             `gorm:"column:sort_order;not null;default:0;index:idx_word_id_kind,priority:3"`
}

func (WordMeaning) TableName() string { return "word_meanings" }

// WordAudio 单词发音及本地音频文件元数据
type WordAudio struct {
	ID           int64     `gorm:"primaryKey;column:id"`
	WordID       int64     `gorm:"column:word_id;not null;uniqueIndex:uk_word_accent,priority:1;index:idx_word_id"`
	Accent       string    `gorm:"column:accent;size:8;not null;default:'EN';uniqueIndex:uk_word_accent,priority:2"`
	PhoneticText string    `gorm:"column:phonetic_text;size:128;not null;default:''"`
	SourceURL    string    `gorm:"column:source_url;size:512;not null;uniqueIndex:uk_source_url"`
	FilePath     string    `gorm:"column:file_path;size:255;not null"`
	ContentType  string    `gorm:"column:content_type;size:64;not null;default:'audio/mpeg'"`
	FileSize     int64     `gorm:"column:file_size;not null;default:0"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (WordAudio) TableName() string { return "word_audios" }

// Phrase 收藏短语主表；短语只有一个美音，不保存音标。
type Phrase struct {
	ID               int64     `gorm:"primaryKey;column:id;index:idx_phrases_created_at,priority:2"`
	Phrase           string    `gorm:"column:phrase;size:80;uniqueIndex:uk_phrase;not null"`
	AudioFilePath    string    `gorm:"column:audio_file_path;size:255;not null"`
	AudioContentType string    `gorm:"column:audio_content_type;size:64;not null;default:'audio/mpeg'"`
	AudioFileSize    int64     `gorm:"column:audio_file_size;not null;default:0"`
	CreatedAt        time.Time `gorm:"column:created_at;autoCreateTime;index:idx_phrases_created_at,priority:1"`
	UpdatedAt        time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (Phrase) TableName() string { return "phrases" }

// PhraseMeaning 短语中文释义。
type PhraseMeaning struct {
	ID           int64  `gorm:"primaryKey;column:id"`
	PhraseID     int64  `gorm:"column:phrase_id;not null;index:idx_phrase_meaning,priority:1"`
	PartOfSpeech string `gorm:"column:part_of_speech;size:32;not null;default:''"`
	Definition   string `gorm:"column:definition;type:text;not null"`
	SortOrder    int    `gorm:"column:sort_order;not null;default:0;index:idx_phrase_meaning,priority:2"`
}

func (PhraseMeaning) TableName() string { return "phrase_meanings" }

// SentenceFavorite 保存用户收藏的英文句子及其中文翻译，不参与学习计划。
type SentenceFavorite struct {
	ID          int64     `gorm:"primaryKey;column:id;index:idx_sentence_favorites_created_at,priority:2"`
	Sentence    string    `gorm:"column:sentence;type:text;not null"`
	Translation string    `gorm:"column:translation;type:text;not null"`
	Note        string    `gorm:"column:note;size:512;not null;default:''"`
	DedupeKey   string    `gorm:"column:dedupe_key;size:64;not null;uniqueIndex:uk_sentence_favorite_dedupe"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime;index:idx_sentence_favorites_created_at,priority:1"`
}

func (SentenceFavorite) TableName() string { return "sentence_favorites" }

type SentenceTag struct {
	ID        int64     `gorm:"primaryKey;column:id"`
	Name      string    `gorm:"column:name;size:64;not null;uniqueIndex:uk_sentence_tag_name"`
	Color     string    `gorm:"column:color;size:7;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (SentenceTag) TableName() string { return "sentence_tags" }

type SentenceFavoriteTag struct {
	SentenceFavoriteID int64     `gorm:"primaryKey;column:sentence_favorite_id;index:idx_sentence_favorite_tags_tag,priority:2"`
	TagID              int64     `gorm:"primaryKey;column:tag_id;index:idx_sentence_favorite_tags_tag,priority:1"`
	CreatedAt          time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (SentenceFavoriteTag) TableName() string { return "sentence_favorite_tags" }

// ItemNote 单词或短语的独立备注，不依赖收藏实体生命周期。
type ItemNote struct {
	ID        int64     `gorm:"primaryKey;column:id"`
	ItemType  string    `gorm:"column:item_type;size:16;not null;uniqueIndex:uk_item_note,priority:1"`
	ItemText  string    `gorm:"column:item_text;size:80;not null;uniqueIndex:uk_item_note,priority:2"`
	Note      string    `gorm:"column:note;type:text;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (ItemNote) TableName() string { return "item_notes" }

// Context 通用语境，通过 ItemType + ItemID 关联具体收藏项。
type Context struct {
	ID               int64     `gorm:"primaryKey;column:id"`
	ItemType         string    `gorm:"column:item_type;size:16;not null;index:idx_context_item,priority:1"`
	ItemID           int64     `gorm:"column:item_id;not null;index:idx_context_item,priority:2"`
	Sentence         string    `gorm:"column:sentence;type:text;not null"`
	Translation      string    `gorm:"column:translation;type:text;not null"`
	HighlightStart   int       `gorm:"column:highlight_start;not null"`
	HighlightEnd     int       `gorm:"column:highlight_end;not null"`
	AudioFilePath    string    `gorm:"column:audio_file_path;size:255;not null"`
	AudioContentType string    `gorm:"column:audio_content_type;size:64;not null;default:'audio/mpeg'"`
	AudioFileSize    int64     `gorm:"column:audio_file_size;not null;default:0"`
	DedupeKey        string    `gorm:"column:dedupe_key;size:64;not null;uniqueIndex:uk_context_dedupe"`
	CreatedAt        time.Time `gorm:"column:created_at;autoCreateTime;index:idx_context_item,priority:3"`
}

func (Context) TableName() string { return "contexts" }

// ContextVideo 是一次语境在本地视频中的出现记录。
type ContextVideo struct {
	ID                int64     `gorm:"primaryKey;column:id"`
	ContextID         int64     `gorm:"column:context_id;not null;index:idx_context_video_context,priority:1"`
	SourceFingerprint string    `gorm:"column:source_fingerprint;size:64;not null"`
	ClipStartMS       int64     `gorm:"column:clip_start_ms;not null"`
	ClipEndMS         int64     `gorm:"column:clip_end_ms;not null"`
	DurationMS        int64     `gorm:"column:duration_ms;not null"`
	FilePath          string    `gorm:"column:file_path;size:255;not null"`
	ContentType       string    `gorm:"column:content_type;size:64;not null;default:'video/webm'"`
	FileSize          int64     `gorm:"column:file_size;not null;default:0"`
	SubtitlesJSON     string    `gorm:"column:subtitles_json;type:mediumtext;not null"`
	DedupeKey         string    `gorm:"column:dedupe_key;size:64;not null;uniqueIndex:uk_context_video_dedupe"`
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime;index:idx_context_video_context,priority:2"`
}

func (ContextVideo) TableName() string { return "context_videos" }

// ContextTask 语境同步处理任务。
type ContextTask struct {
	ID                      int64      `gorm:"primaryKey;column:id"`
	DedupeKey               string     `gorm:"column:dedupe_key;size:64;not null;uniqueIndex:uk_context_task_dedupe"`
	ItemType                string     `gorm:"column:item_type;size:16;not null;index:idx_context_task_item,priority:1"`
	ItemID                  int64      `gorm:"column:item_id;not null;index:idx_context_task_item,priority:2"`
	RawParagraph            string     `gorm:"column:raw_paragraph;type:mediumtext;not null"`
	RawSelectionStart       int        `gorm:"column:raw_selection_start;not null"`
	RawSelectionEnd         int        `gorm:"column:raw_selection_end;not null"`
	SplitterVersion         string     `gorm:"column:splitter_version;size:32;not null"`
	ExtractedSentence       *string    `gorm:"column:extracted_sentence;type:text"`
	ExtractedSentenceStart  *int       `gorm:"column:extracted_sentence_start"`
	ExtractedSentenceEnd    *int       `gorm:"column:extracted_sentence_end"`
	ExtractedHighlightStart *int       `gorm:"column:extracted_highlight_start"`
	ExtractedHighlightEnd   *int       `gorm:"column:extracted_highlight_end"`
	Status                  string     `gorm:"column:status;size:16;not null;index:idx_context_task_status,priority:1"`
	Attempts                int        `gorm:"column:attempts;not null;default:0"`
	LastError               string     `gorm:"column:last_error;type:text;not null"`
	ResultContextID         *int64     `gorm:"column:result_context_id"`
	StartedAt               *time.Time `gorm:"column:started_at"`
	FinishedAt              *time.Time `gorm:"column:finished_at"`
	CreatedAt               time.Time  `gorm:"column:created_at;autoCreateTime;index:idx_context_task_item,priority:3"`
	UpdatedAt               time.Time  `gorm:"column:updated_at;autoUpdateTime;index:idx_context_task_status,priority:2"`
}

func (ContextTask) TableName() string { return "context_tasks" }

type UserItemLearning struct {
	ID                int64      `gorm:"primaryKey;column:id"`
	ItemType          int        `gorm:"column:item_type;not null;uniqueIndex:uk_learning_item,priority:1"`
	ItemID            int64      `gorm:"column:item_id;not null;uniqueIndex:uk_learning_item,priority:2"`
	QueuedAt          time.Time  `gorm:"column:queued_at;type:datetime(6);not null;index:idx_learning_queued"`
	LearningStatus    string     `gorm:"column:learning_status;size:20;not null;index:idx_learning_due,priority:1"`
	MemoryStage       int        `gorm:"column:memory_stage;not null;default:0"`
	ReviewSuccessDays int        `gorm:"column:review_success_days;not null;default:0"`
	LastReviewedAt    *time.Time `gorm:"column:last_reviewed_at"`
	NextReviewAt      *time.Time `gorm:"column:next_review_at;index:idx_learning_due,priority:2"`
	ScheduleVersion   int        `gorm:"column:schedule_version;not null;default:1"`
	LearnedAt         *time.Time `gorm:"column:learned_at"`
	MasteredAt        *time.Time `gorm:"column:mastered_at"`
	CreatedAt         time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (UserItemLearning) TableName() string { return "user_item_learning" }

type LearningSession struct {
	ID                 int64      `gorm:"primaryKey;column:id"`
	StudyDate          time.Time  `gorm:"column:study_date;type:date;not null;index:idx_learning_session_date,priority:1"`
	Status             string     `gorm:"column:status;size:20;not null;index:idx_learning_session_date,priority:2"`
	TurnNo             int        `gorm:"column:turn_no;not null;default:0"`
	CurrentQueueItemID *int64     `gorm:"column:current_queue_item_id"`
	CreatedAt          time.Time  `gorm:"column:created_at;autoCreateTime"`
	CompletedAt        *time.Time `gorm:"column:completed_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (LearningSession) TableName() string { return "learning_sessions" }

type UserLearningQueue struct {
	ID              int64      `gorm:"primaryKey;column:id"`
	SessionID       int64      `gorm:"column:session_id;not null;uniqueIndex:uk_session_item,priority:1;index:idx_queue_pick,priority:1"`
	ItemType        int        `gorm:"column:item_type;not null;uniqueIndex:uk_session_item,priority:2"`
	ItemID          int64      `gorm:"column:item_id;not null;uniqueIndex:uk_session_item,priority:3"`
	QueueType       string     `gorm:"column:queue_type;size:20;not null"`
	SourceQueueType string     `gorm:"column:source_queue_type;size:20;not null"`
	QueueSeq        float64    `gorm:"column:queue_seq;type:decimal(10,2);not null;index:idx_queue_pick,priority:4"`
	Status          string     `gorm:"column:status;size:20;not null;index:idx_queue_pick,priority:2"`
	HadIncorrect    bool       `gorm:"column:had_incorrect;not null;default:false"`
	IncorrectCount  int        `gorm:"column:incorrect_count;not null;default:0"`
	AnswerCount     int        `gorm:"column:answer_count;not null;default:0"`
	KnownCount      int        `gorm:"column:known_count;not null;default:0"`
	RetryAfterTurn  *int       `gorm:"column:retry_after_turn;index:idx_queue_pick,priority:3"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime"`
	CompletedAt     *time.Time `gorm:"column:completed_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (UserLearningQueue) TableName() string { return "user_learning_queue" }

// LearningSettings 是当前单用户系统的全局学习计划设置。
type LearningSettings struct {
	ID               int       `gorm:"primaryKey;column:id"`
	DailyNewLimit    int       `gorm:"column:daily_new_limit;not null"`
	DailyReviewLimit int       `gorm:"column:daily_review_limit;not null"`
	CreatedAt        time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt        time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (LearningSettings) TableName() string { return "learning_settings" }

// ItemActionEvent 是面向行为分析的追加式事件。收藏删除后仍保留文本快照。
type ItemActionEvent struct {
	ID          int64     `gorm:"primaryKey;column:id"`
	EventID     string    `gorm:"column:event_id;size:64;not null;uniqueIndex:uk_item_action_event_id"`
	ItemType    string    `gorm:"column:item_type;size:16;not null;index:idx_item_action_target,priority:1"`
	ItemID      int64     `gorm:"column:item_id;not null;index:idx_item_action_target,priority:2"`
	ItemText    string    `gorm:"column:item_text;size:80;not null"`
	Action      string    `gorm:"column:action;size:32;not null;index:idx_item_action_action,priority:1"`
	Source      string    `gorm:"column:source;size:32;not null;index:idx_item_action_source,priority:1"`
	LevelBefore *int      `gorm:"column:level_before"`
	LevelAfter  *int      `gorm:"column:level_after"`
	SessionID   *int64    `gorm:"column:session_id"`
	QueueItemID *int64    `gorm:"column:queue_item_id"`
	ContextID   *int64    `gorm:"column:context_id"`
	Metadata    *string   `gorm:"column:metadata;type:json"`
	OccurredAt  time.Time `gorm:"column:occurred_at;type:datetime(6);not null;index:idx_item_action_target,priority:3;index:idx_item_action_action,priority:2;index:idx_item_action_source,priority:2"`
	CreatedAt   time.Time `gorm:"column:created_at;type:datetime(6);autoCreateTime"`
}

func (ItemActionEvent) TableName() string { return "item_action_events" }

// WordGroup 相似单词组
type WordGroup struct {
	ID          int64     `gorm:"primaryKey;column:id"`
	MemberCount int       `gorm:"column:member_count;not null;default:0"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (WordGroup) TableName() string { return "word_groups" }

// WordGroupMember 相似单词组成员：一个词只能归属一个组
type WordGroupMember struct {
	ID      int64     `gorm:"primaryKey;column:id"`
	GroupID int64     `gorm:"column:group_id;not null;index:idx_group_id"`
	WordID  int64     `gorm:"column:word_id;not null;uniqueIndex:uk_word_id"`
	AddedAt time.Time `gorm:"column:added_at;autoCreateTime"`
}

func (WordGroupMember) TableName() string { return "word_group_members" }

// AutoMigrate 便捷方法：DB 已注入时可用于验证 schema 与 model 一致
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&Word{}, &WordMeaning{}, &WordAudio{}, &Phrase{}, &PhraseMeaning{}, &SentenceFavorite{}, &SentenceTag{}, &SentenceFavoriteTag{}, &ItemNote{}, &Context{}, &ContextVideo{}, &ContextTask{},
		&UserItemLearning{}, &LearningSession{}, &UserLearningQueue{}, &LearningSettings{},
		&ItemActionEvent{},
		&WordGroup{}, &WordGroupMember{},
	)
}
