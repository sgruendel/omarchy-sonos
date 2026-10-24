import QtQuick
import Quickshell
import Quickshell.Io

Item {
  id: root
  property var shell: null
  property var manifest: null
  property var snapshot: ({status: "starting", rooms: [], playback: {}, account: {}, rp: {}})
  property string lastError: ""
  property string lastDiagnostic: ""
  property bool busy: false
  property int requestCounter: 0
  property var pending: ({})
  property var completedResults: []
  property int restartAttempt: 0
  property bool stopping: false
  readonly property bool ready: backend.running && snapshot.status === "ready"
  readonly property string backendPath: decodeURIComponent(String(Qt.resolvedUrl("sonos-backend")).replace(/^file:\/\//, ""))
  // The backend emits a result followed by its refreshed snapshot. Finish only
  // after both have arrived so callers can use the speaker's actual state.
  signal commandFinished(string requestId, bool ok)
  property alias volume: roomVolume

  VolumeController { id: roomVolume; service: root }

  function send(op, args) {
    if (!backend.running) { lastError = "Backend is unavailable"; return "" }
    var payload = {id: String(++requestCounter), op: op}
    var fields = args || {}
    for (var key in fields) payload[key] = fields[key]
    var next = {}
    for (var id in pending) next[id] = true
    next[payload.id] = true
    pending = next
    busy = true
    lastError = ""
    backend.write(JSON.stringify(payload) + "\n")
    return payload.id
  }
  function receive(line) {
    try {
      var message = JSON.parse(line)
      if (message.type === "snapshot") {
        snapshot = message
        restartAttempt = 0
        var results = completedResults
        completedResults = []
        var remaining = {}
        for (var requestId in pending) remaining[requestId] = true
        for (var i = 0; i < results.length; ++i) delete remaining[results[i].id]
        pending = remaining
        busy = Object.keys(pending).length > 0
        for (var j = 0; j < results.length; ++j)
          commandFinished(results[j].id, results[j].ok)
      } else if (message.type === "result") {
        if (pending[message.id]) completedResults = completedResults.concat([{id: message.id, ok: message.ok === true}])
        if (!message.ok) lastError = String(message.error || "Command failed")
      }
    } catch (e) { lastError = "Invalid backend response" }
  }
  Process {
    id: backend
    command: [root.backendPath]
    stdinEnabled: true
    stdout: SplitParser { onRead: function(line) { root.receive(line) } }
    // Never print backend input or authentication payloads to the shell log.
    stderr: SplitParser { onRead: function(line) { root.lastDiagnostic = String(line) } }
    onExited: function(code) {
      root.busy = false
      root.pending = ({})
      root.completedResults = []
      root.lastDiagnostic = ""
      root.snapshot = ({status: "offline", rooms: [], playback: {}, account: {}, rp: {}})
      if (!root.stopping) {
        root.lastError = root.lastError || "Backend stopped (" + code + ")"
        restart.interval = Math.min(30000, 1000 * Math.pow(2, root.restartAttempt++))
        restart.restart()
      }
    }
  }
  Timer { id: restart; onTriggered: if (!root.stopping) backend.running = true }
  Component.onCompleted: backend.running = true
  Component.onDestruction: { stopping = true; backend.running = false }
}
