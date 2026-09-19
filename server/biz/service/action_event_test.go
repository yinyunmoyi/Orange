package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"aaa_word/biz/db"
	"aaa_word/biz/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRecordClientItemActionIncrementsWordOnce(t *testing.T) {
	gdb := installActionEventTestDB(t)
	word := db.Word{Word: "example"}
	if err := gdb.Create(&word).Error; err != nil {
		t.Fatal(err)
	}
	if word.Level != 1 {
		t.Fatalf("new word level = %d, want 1", word.Level)
	}
	req := &model.ItemActionRequest{
		EventID: "lookup-event-0001",
		Action:  model.ActionLookupOpened,
		Source:  model.ActionSourceAndroidReader,
	}
	first, err := RecordClientItemAction(context.Background(), ContextItemWord, word.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RecordClientItemAction(context.Background(), ContextItemWord, word.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Level != 2 || second.Level != 2 {
		t.Fatalf("levels = (%d, %d), want (2, 2)", first.Level, second.Level)
	}
	var events int64
	if err := gdb.Model(&db.ItemActionEvent{}).Count(&events).Error; err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("event count = %d, want 1", events)
	}
}

func TestRecordClientItemActionPhraseDoesNotHaveLevel(t *testing.T) {
	gdb := installActionEventTestDB(t)
	phrase := db.Phrase{Phrase: "look up"}
	if err := gdb.Create(&phrase).Error; err != nil {
		t.Fatal(err)
	}
	data, err := RecordClientItemAction(context.Background(), ContextItemPhrase, phrase.ID, &model.ItemActionRequest{
		EventID: "lookup-event-0002",
		Action:  model.ActionLookupOpened,
		Source:  model.ActionSourceWebVideo,
	})
	if err != nil {
		t.Fatal(err)
	}
	if data.Level != 0 {
		t.Fatalf("phrase level = %d, want 0", data.Level)
	}
}

func TestRecordClientItemActionRejectsInvalidAction(t *testing.T) {
	installActionEventTestDB(t)
	_, err := RecordClientItemAction(context.Background(), ContextItemWord, 1, &model.ItemActionRequest{
		EventID: "invalid-event-01",
		Action:  model.ActionLearningUnknown,
		Source:  model.ActionSourceAndroidReader,
	})
	if !errors.Is(err, ErrItemActionInvalid) {
		t.Fatalf("error = %v, want %v", err, ErrItemActionInvalid)
	}
}

func TestLearningAnswerLevelRules(t *testing.T) {
	gdb := installActionEventTestDB(t)
	word := db.Word{Word: "difficult"}
	if err := gdb.Create(&word).Error; err != nil {
		t.Fatal(err)
	}
	queue := db.UserLearningQueue{ID: 9, ItemType: LearningItemWord, ItemID: word.ID}
	if err := gdb.Transaction(func(tx *gorm.DB) error {
		return recordLearningAnswerAction(tx, &queue, 3, 1, false, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if err := gdb.Transaction(func(tx *gorm.DB) error {
		return recordLearningAnswerAction(tx, &queue, 3, 2, true, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if err := gdb.First(&word, word.ID).Error; err != nil {
		t.Fatal(err)
	}
	if word.Level != 2 {
		t.Fatalf("word level = %d, want 2", word.Level)
	}
	var events []db.ItemActionEvent
	if err := gdb.Select("action").Order("id").Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 ||
		events[0].Action != model.ActionLearningUnknown ||
		events[1].Action != model.ActionLearningKnown {
		t.Fatalf("events = %+v", events)
	}
}

func TestResetLearningKeepsWordLevel(t *testing.T) {
	gdb := installActionEventTestDB(t)
	word := db.Word{Word: "retain", Level: 6}
	if err := gdb.Create(&word).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Create(&db.UserItemLearning{
		ItemType: LearningItemWord, ItemID: word.ID, QueuedAt: time.Now(),
		LearningStatus: "learning",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := ResetFavoriteLearning(context.Background(), ContextItemWord, word.ID); err != nil {
		t.Fatal(err)
	}
	if err := gdb.First(&word, word.ID).Error; err != nil {
		t.Fatal(err)
	}
	if word.Level != 6 {
		t.Fatalf("word level after reset = %d, want 6", word.Level)
	}
}

func TestActionEventSurvivesFavoriteDeletion(t *testing.T) {
	gdb := installActionEventTestDB(t)
	word := db.Word{Word: "history"}
	if err := gdb.Create(&word).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := RecordClientItemAction(context.Background(), ContextItemWord, word.ID, &model.ItemActionRequest{
		EventID: "detail-event-0001",
		Action:  model.ActionDetailOpened,
		Source:  model.ActionSourceAndroidWordDetail,
	}); err != nil {
		t.Fatal(err)
	}
	if err := gdb.Delete(&word).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := gdb.Model(&db.ItemActionEvent{}).Where("item_id = ?", word.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("retained event count = %d, want 1", count)
	}
}

func installActionEventTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	old := db.DB
	gdb, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.AutoMigrate(
		&db.Word{}, &db.Phrase{}, &db.ItemActionEvent{},
		&db.UserItemLearning{}, &db.UserLearningQueue{}, &db.LearningSession{},
	); err != nil {
		t.Fatal(err)
	}
	db.DB = gdb
	t.Cleanup(func() {
		db.DB = old
		sqlDB, _ := gdb.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	})
	return gdb
}
