package com.example.orange.data.reverselan

import com.example.orange.data.network.NetworkAccessPolicy
import com.example.orange.data.word.WordServiceConfig
import java.io.IOException
import java.net.URL
import org.json.JSONObject

internal object ReverseLanRegistrationApi {
    fun register(
        clientId: String,
        serverId: String,
        generation: String,
        secret: String,
    ) {
        NetworkAccessPolicy.requireAllowed()
        val connection = NetworkAccessPolicy.openConnection(
            URL("${WordServiceConfig.origin}/api/v1/reverse-lan/registrations"),
        )
        try {
            connection.requestMethod = "POST"
            connection.connectTimeout = 8_000
            connection.readTimeout = 8_000
            connection.instanceFollowRedirects = false
            connection.doOutput = true
            connection.setRequestProperty("Accept", "application/json")
            connection.setRequestProperty("Content-Type", "application/json; charset=utf-8")
            val body = JSONObject()
                .put("apiVersion", REVERSE_LAN_API_VERSION)
                .put("clientId", clientId)
                .put("serverId", serverId)
                .put("generation", generation)
                .put("secret", secret)
                .toString()
            connection.outputStream.use { output ->
                output.write(body.toByteArray(Charsets.UTF_8))
            }
            val status = connection.responseCode
            val response = if (status in 200..299) {
                connection.inputStream.use { it.reader(Charsets.UTF_8).readText() }
            } else {
                connection.errorStream?.use { it.reader(Charsets.UTF_8).readText() }.orEmpty()
            }
            if (status !in 200..299) {
                throw IOException("reverse LAN registration HTTP $status")
            }
            val root = JSONObject(response)
            if (root.optInt("code", -1) != 0) {
                throw IOException("reverse LAN registration rejected")
            }
        } finally {
            NetworkAccessPolicy.release(connection)
        }
    }
}
