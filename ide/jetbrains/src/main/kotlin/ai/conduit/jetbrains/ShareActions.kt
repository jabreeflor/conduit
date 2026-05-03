package ai.conduit.jetbrains

import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.CommonDataKeys
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.ui.Messages
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.time.Duration

private fun postContext(payload: String): Boolean {
    val state = ConnectionStatus.get()
    if (state !is ConnectionState.Connected) return false
    return try {
        val req = HttpRequest.newBuilder(URI.create("${state.endpoint}/v1/context"))
            .timeout(Duration.ofSeconds(2))
            .header("content-type", "application/json")
            .POST(HttpRequest.BodyPublishers.ofString(payload))
            .build()
        val resp = HttpClient.newHttpClient().send(req, HttpResponse.BodyHandlers.discarding())
        resp.statusCode() in 200..299
    } catch (_: Exception) {
        false
    }
}

class ShareFileAction : AnAction() {
    override fun actionPerformed(e: AnActionEvent) {
        val file = e.getData(CommonDataKeys.VIRTUAL_FILE) ?: return
        val payload = """{"kind":"file","uri":"${file.url}"}"""
        ApplicationManager.getApplication().executeOnPooledThread {
            val ok = postContext(payload)
            ApplicationManager.getApplication().invokeLater {
                if (!ok) Messages.showWarningDialog(
                    "Conduit is not connected, or the share request failed.",
                    "Conduit",
                )
            }
        }
    }
}

class ShareSelectionAction : AnAction() {
    override fun actionPerformed(e: AnActionEvent) {
        val editor = e.getData(CommonDataKeys.EDITOR) ?: return
        val file = e.getData(CommonDataKeys.VIRTUAL_FILE) ?: return
        val sel = editor.selectionModel
        if (!sel.hasSelection()) return
        val doc = editor.document
        val startLine = doc.getLineNumber(sel.selectionStart)
        val endLine = doc.getLineNumber(sel.selectionEnd)
        val payload = """{"kind":"selection","uri":"${file.url}","startLine":$startLine,"endLine":$endLine}"""
        ApplicationManager.getApplication().executeOnPooledThread {
            val ok = postContext(payload)
            ApplicationManager.getApplication().invokeLater {
                if (!ok) Messages.showWarningDialog(
                    "Conduit is not connected, or the share request failed.",
                    "Conduit",
                )
            }
        }
    }
}
