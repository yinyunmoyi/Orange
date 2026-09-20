package com.example.orange.data.learning

import com.example.orange.data.logging.AppLog as Log
import com.example.orange.data.word.WordServiceConfig
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL

object ReviewForecastApi {
    suspend fun get(days: Int = 14): Result<LearningReviewForecast> = withContext(Dispatchers.IO) {
        runCatching {
            val url = "${WordServiceConfig.origin}/api/v1/learning/review-forecast?days=$days"
            val json = getJson(url)
            val data = json.optJSONObject("data")
                ?: throw IOException(json.optString("msg", "response missing data"))
            val items = mutableListOf<LearningReviewForecastItem>()
            data.optJSONArray("items")?.let { array ->
                for (index in 0 until array.length()) {
                    val item = array.optJSONObject(index) ?: continue
                    val date = item.optString("date").trim()
                    val count = item.optInt("count", -1)
                    if (date.isEmpty() || count < 0) {
                        Log.w(TAG, "skip invalid item index=$index date=$date count=$count")
                        continue
                    }
                    items += LearningReviewForecastItem(date, count)
                }
            }
            LearningReviewForecast(
                startDate = data.optString("startDate"),
                endDate = data.optString("endDate"),
                days = data.optInt("days", days),
                total = data.optInt("total"),
                items = items,
            )
        }
    }

    private fun getJson(urlString: String): JSONObject {
        val started = System.currentTimeMillis()
        val connection = URL(urlString).openConnection() as HttpURLConnection
        try {
            connection.requestMethod = "GET"
            connection.connectTimeout = 8_000
            connection.readTimeout = 15_000
            connection.setRequestProperty("Accept", "application/json")
            connection.setRequestProperty("Connection", "close")
            val code = connection.responseCode
            val text = if (code in 200..299) {
                connection.inputStream.use { it.reader(Charsets.UTF_8).readText() }
            } else {
                connection.errorStream?.use { it.reader(Charsets.UTF_8).readText() }.orEmpty()
            }
            if (code !in 200..299) {
                Log.w(TAG, "GET failed url=$urlString code=$code body=${text.take(500)}")
                throw IOException("HTTP $code ${runCatching { JSONObject(text).optString("msg") }.getOrDefault("")}")
            }
            return JSONObject(text)
        } catch (error: Throwable) {
            Log.e(TAG, "GET error url=$urlString elapsed=${System.currentTimeMillis() - started}ms", error)
            throw error
        } finally {
            connection.disconnect()
        }
    }

    private const val TAG = "ReviewForecastApi"
}
