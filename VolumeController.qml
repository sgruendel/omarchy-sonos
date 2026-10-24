import QtQuick

// Shared by all bar instances. Keep one volume request in flight and retain
// only the latest input while it (or another backend command) is running.
Item {
  id: root
  required property var service
  readonly property bool ready: !!service && service.ready
  readonly property string contextKey: service ? (service.snapshot.selected || "") : ""
  readonly property int sourceValue: service ? (service.snapshot.volume || 0) : 0
  readonly property int displayValue: previewValue >= 0 ? previewValue : sourceValue
  property int previewValue: -1
  property int queuedValue: -1
  property string requestId: ""

  function reset() {
    debounce.stop()
    queuedValue = -1
    previewValue = -1
    requestId = ""
  }
  function setVolume(value) {
    if (!ready) return
    queuedValue = Math.max(0, Math.min(100, Math.round(value)))
    previewValue = queuedValue
    if (!requestId && queuedValue === sourceValue) {
      reset()
      return
    }
    debounce.restart()
  }
  function adjust(delta) { setVolume(displayValue + delta) }
  function flush() {
    if (!ready || service.busy || requestId || queuedValue < 0 || debounce.running) return
    var desired = queuedValue
    queuedValue = -1
    if (desired === sourceValue) {
      previewValue = -1
      return
    }
    requestId = service.send("setVolume", {volume: desired}) || ""
    if (!requestId) reset()
  }
  onContextKeyChanged: reset()
  onReadyChanged: if (!ready) reset()

  Timer { id: debounce; interval: 120; onTriggered: root.flush() }
  Connections {
    target: root.service
    function onBusyChanged() { if (!root.service.busy) root.flush() }
    function onCommandFinished(id, ok) {
      if (id !== root.requestId) return
      root.requestId = ""
      // The snapshot is authoritative even when Sonos capped the requested
      // volume or the command failed. Newer user input keeps its preview.
      if (root.queuedValue >= 0) root.flush()
      else root.previewValue = -1
    }
  }
}
