package ai.conduit.jetbrains

import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.time.Duration

/**
 * Connection state for the Conduit daemon. Mirrors the VS Code extension's
 * shape so logs and surfaces stay legible across editors.
 */
sealed class ConnectionState {
    object Disconnected : ConnectionState()
    data class Connecting(val endpoint: String) : ConnectionState()
    data class Connected(val endpoint: String, val version: String) : ConnectionState()
    data class Error(val endpoint: String, val message: String) : ConnectionState()
}

/**
 * Hits `<endpoint>/v1/healthz` and normalizes every failure mode into a
 * [ConnectionState.Error]. Exposes [client] and [bodyParser] for tests so we
 * never touch a real socket from a unit test.
 */
class Probe(
    private val client: HttpFetcher = JdkHttpFetcher(),
    private val bodyParser: BodyParser = SimpleHealthBodyParser,
) {
    fun probe(endpoint: String, timeout: Duration): ConnectionState {
        val url = endpoint.trimEnd('/') + "/v1/healthz"
        return try {
            val resp = client.get(URI.create(url), timeout)
            if (resp.statusCode !in 200..299) {
                return ConnectionState.Error(endpoint, "HTTP ${resp.statusCode}")
            }
            val parsed = bodyParser.parse(resp.body)
                ?: return ConnectionState.Error(endpoint, "malformed /v1/healthz body")
            ConnectionState.Connected(endpoint, parsed.version)
        } catch (e: java.net.http.HttpTimeoutException) {
            ConnectionState.Error(endpoint, "timed out after ${timeout.toMillis()}ms")
        } catch (e: Exception) {
            ConnectionState.Error(endpoint, e.message ?: e.javaClass.simpleName)
        }
    }
}

data class HealthBody(val status: String, val version: String)

interface BodyParser { fun parse(raw: String): HealthBody? }

/**
 * Tiny hand-rolled parser for the two fields we care about. Keeps the
 * scaffold dependency-free; the chat-panel PR can adopt a real JSON lib.
 */
object SimpleHealthBodyParser : BodyParser {
    private val statusRx = Regex("\"status\"\\s*:\\s*\"([^\"]+)\"")
    private val versionRx = Regex("\"version\"\\s*:\\s*\"([^\"]+)\"")

    override fun parse(raw: String): HealthBody? {
        val status = statusRx.find(raw)?.groupValues?.get(1) ?: return null
        val version = versionRx.find(raw)?.groupValues?.get(1) ?: "unknown"
        return HealthBody(status, version)
    }
}

interface HttpFetcher {
    data class Response(val statusCode: Int, val body: String)
    fun get(uri: URI, timeout: Duration): Response
}

class JdkHttpFetcher : HttpFetcher {
    private val client: HttpClient = HttpClient.newHttpClient()
    override fun get(uri: URI, timeout: Duration): HttpFetcher.Response {
        val req = HttpRequest.newBuilder(uri).timeout(timeout).GET().build()
        val resp = client.send(req, HttpResponse.BodyHandlers.ofString())
        return HttpFetcher.Response(resp.statusCode(), resp.body())
    }
}
