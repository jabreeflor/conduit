package ai.conduit.jetbrains

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.components.PersistentStateComponent
import com.intellij.openapi.components.Service
import com.intellij.openapi.components.State
import com.intellij.openapi.components.Storage
import com.intellij.util.xmlb.XmlSerializerUtil

@Service(Service.Level.APP)
@State(name = "ConduitSettings", storages = [Storage("conduit.xml")])
class ConduitSettings : PersistentStateComponent<ConduitSettings.State> {
    data class State(
        var endpoint: String = "http://127.0.0.1:8923",
        var timeoutMs: Long = 2000,
    )

    private var state = State()

    override fun getState(): State = state
    override fun loadState(s: State) { XmlSerializerUtil.copyBean(s, state) }

    val endpoint: String get() = state.endpoint
    val timeoutMs: Long get() = state.timeoutMs

    companion object {
        fun get(): ConduitSettings = ApplicationManager.getApplication().getService(ConduitSettings::class.java)
    }
}
