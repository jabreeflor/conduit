package ai.conduit.jetbrains

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.application.ApplicationManager
import java.time.Duration

class ConnectAction : AnAction() {
    private val probe = Probe()

    override fun actionPerformed(e: AnActionEvent) {
        val settings = ConduitSettings.get()
        val endpoint = settings.endpoint
        val timeout = Duration.ofMillis(settings.timeoutMs)

        ApplicationManager.getApplication().executeOnPooledThread {
            val state = probe.probe(endpoint, timeout)
            ApplicationManager.getApplication().invokeLater {
                ConnectionStatus.set(state)
                notify(state)
            }
        }
    }

    private fun notify(state: ConnectionState) {
        val (msg, type) = when (state) {
            is ConnectionState.Connected -> "Conduit connected (v${state.version}) at ${state.endpoint}" to NotificationType.INFORMATION
            is ConnectionState.Error -> "Could not reach Conduit at ${state.endpoint}: ${state.message}" to NotificationType.WARNING
            else -> return
        }
        NotificationGroupManager.getInstance()
            .getNotificationGroup("Conduit")
            .createNotification(msg, type)
            .notify(null)
    }
}

/** Process-wide connection state, observable by the tool window and actions. */
object ConnectionStatus {
    @Volatile
    private var current: ConnectionState = ConnectionState.Disconnected
    fun get(): ConnectionState = current
    fun set(s: ConnectionState) { current = s }
}
