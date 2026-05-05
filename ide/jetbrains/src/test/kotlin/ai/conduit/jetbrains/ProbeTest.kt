package ai.conduit.jetbrains

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertInstanceOf
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.net.URI
import java.time.Duration

private class FakeHttpFetcher(
    private val statusCode: Int = 200,
    private val body: String = """{"status":"ok","version":"0.1.0"}""",
    private val throwIt: Exception? = null,
) : HttpFetcher {
    override fun get(uri: URI, timeout: Duration): HttpFetcher.Response {
        throwIt?.let { throw it }
        return HttpFetcher.Response(statusCode, body)
    }
}

class ProbeTest {

    @Test
    fun connectsOnHealthyEndpoint() {
        val state = Probe(client = FakeHttpFetcher()).probe("http://localhost:8923", Duration.ofMillis(100))
        val connected = assertInstanceOf(ConnectionState.Connected::class.java, state)
        assertEquals("0.1.0", connected.version)
    }

    @Test
    fun errorsOnNon2xx() {
        val state = Probe(client = FakeHttpFetcher(statusCode = 500)).probe("http://x", Duration.ofMillis(100))
        val err = assertInstanceOf(ConnectionState.Error::class.java, state)
        assertTrue(err.message.contains("HTTP 500"))
    }

    @Test
    fun errorsOnMalformedBody() {
        val state = Probe(client = FakeHttpFetcher(body = "{not json}")).probe("http://x", Duration.ofMillis(100))
        val err = assertInstanceOf(ConnectionState.Error::class.java, state)
        assertTrue(err.message.contains("malformed"))
    }

    @Test
    fun errorsOnException() {
        val state = Probe(client = FakeHttpFetcher(throwIt = RuntimeException("ECONNREFUSED")))
            .probe("http://x", Duration.ofMillis(100))
        val err = assertInstanceOf(ConnectionState.Error::class.java, state)
        assertTrue(err.message.contains("ECONNREFUSED"))
    }

    @Test
    fun parsesPlainHealthBody() {
        val parsed = SimpleHealthBodyParser.parse("""{"status":"ok","version":"1.2.3"}""")
        assertEquals("1.2.3", parsed?.version)
    }

    @Test
    fun rejectsBodyWithoutStatus() {
        val parsed = SimpleHealthBodyParser.parse("""{"version":"1.2.3"}""")
        assertEquals(null, parsed)
    }
}
