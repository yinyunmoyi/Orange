package router

import (
	"aaa_word/biz/handler"

	"github.com/cloudwego/hertz/pkg/app/server"
)

// Register 注册所有业务路由
func Register(h *server.Hertz) {
	notes := h.Group("/api/v1/notes")
	{
		notes.GET("", handler.GetNote)
		notes.PUT("", handler.PutNote)
	}

	learning := h.Group("/api/v1/learning")
	{
		learning.GET("/review-forecast", handler.LearningReviewForecast)
		learning.GET("/settings", handler.GetLearningSettings)
		learning.PUT("/settings", handler.PutLearningSettings)
		learning.POST("/sessions", handler.CreateLearningSession)
		learning.GET("/sessions/:id/current", handler.CurrentLearningCard)
		learning.POST("/sessions/:id/answers", handler.SubmitLearningAnswer)
		learning.POST("/sessions/:id/mastery", handler.MasterCurrentLearningItem)
		learning.POST("/sessions/:id/extend", handler.ExtendLearningSession)
	}

	contexts := h.Group("/api/v1/contexts")
	{
		contexts.POST("/tasks", handler.CreateContextTask)
		contexts.GET("/:id/audio", handler.ContextAudio)
		contexts.POST("/:contextId/videos", handler.UploadContextVideo)
	}

	contextVideos := h.Group("/api/v1/context-videos")
	{
		contextVideos.GET("/:videoId/video", handler.ContextVideoFile)
		contextVideos.GET("/:videoId/subtitles.vtt", handler.ContextVideoSubtitles)
	}

	contextVideoSync := h.Group("/api/v1/context-video-sync")
	{
		contextVideoSync.GET("/ping", handler.ContextVideoSyncPing)
		contextVideoSync.GET("/manifest", handler.ContextVideoSyncManifest)
	}

	hlsMedia := h.Group("/api/v1/media/hls")
	{
		hlsMedia.POST("/import", handler.ImportHLSMedia)
		hlsMedia.GET("/:fingerprint/status", handler.HLSMediaStatus)
		hlsMedia.GET("/:fingerprint/master.m3u8", handler.HLSMediaPlaylist)
		hlsMedia.GET("/:fingerprint/:segment", handler.HLSMediaSegment)
	}

	h.POST("/api/v1/debug/log", handler.DebugLog)
	h.POST("/api/v1/debug/android-log", handler.AndroidDebugLog)
	h.POST("/api/v1/logs/web", handler.DebugLog)
	h.POST("/api/v1/logs/android", handler.AndroidDebugLog)

	audioSync := h.Group("/api/v1/audio-sync")
	{
		audioSync.GET("/manifest", handler.AudioSyncManifest)
		audioSync.GET("/items/:itemType/:audioKey", handler.AudioSyncFile)
	}

	sentenceFavorites := h.Group("/api/v1/sentences/favorites")
	{
		sentenceFavorites.POST("", handler.SaveSentenceFavorite)
		sentenceFavorites.GET("", handler.ListSentenceFavorites)
		sentenceFavorites.GET("/status", handler.SentenceFavoriteStatus)
		sentenceFavorites.PUT("/:id/tags", handler.ReplaceSentenceFavoriteTags)
		sentenceFavorites.GET("/:id", handler.GetSentenceFavorite)
	}

	sentenceTags := h.Group("/api/v1/sentence-tags")
	{
		sentenceTags.GET("", handler.ListSentenceTags)
		sentenceTags.POST("", handler.CreateSentenceTag)
	}

	favorites := h.Group("/api/v1/favorites")
	{
		favorites.DELETE("/:itemType/:id", handler.DeleteFavorite)
		favorites.POST("/:itemType/:id/actions", handler.RecordFavoriteAction)
		favorites.POST("/:itemType/:id/audio/regenerate", handler.RegenerateFavoriteAudio)
		favorites.POST("/:itemType/:id/learning/reset", handler.ResetFavoriteLearning)
	}

	phrases := h.Group("/api/v1/phrase")
	{
		phrases.POST("/favorite", handler.PhraseFavorite)
		phrases.GET("/favorite/status", handler.PhraseFavoriteStatus)
		phrases.GET("/:id", handler.PhraseDetail)
		phrases.GET("/:id/audio", handler.PhraseAudio)
	}

	api := h.Group("/api/v1/word")
	{
		api.POST("/lookup", handler.Lookup)
		api.GET("/audio", handler.Audio)
		api.POST("/meaning", handler.Meaning)
		api.POST("/explain", handler.Explain)
		api.POST("/phrase", handler.Phrase)
		api.POST("/contexts", handler.Contexts)
		api.POST("/favorite", handler.Favorite)
		api.GET("/favorite/status", handler.FavoriteStatus)
		api.GET("/favorite-groups", handler.FavoriteGroups)
		api.GET("/favorites", handler.Favorites)
		api.POST("/associations", handler.WordAssociations)
		api.GET("/groups", handler.SimilarGroups)
		api.POST("/groups/members", handler.AddSimilarGroupMember)
		api.GET("/groups/by-word", handler.SimilarGroupByWord)
		api.GET("/groups/:id", handler.SimilarGroupDetail)
		api.POST("/sentence/analyze", handler.AnalyzeSentence)
		api.POST("/sentence/translate", handler.TranslateSentence)
	}
}
