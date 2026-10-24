import QtQuick
import Quickshell

// Run against the real Service and VolumeController, with a backend that emits
// one starting snapshot and then only consumes stdin. No LAN/account requests.
ShellRoot {
  id: root
  property var completions: []
  Service {
    id: service
    onCommandFinished: function(id, ok) {
      root.completions.push({id: id, ok: ok, volume: service.snapshot.volume})
    }
  }
  function check(condition, message) {
    if (!condition) {
      console.error("SERVICE TEST FAILED:", message)
      Qt.exit(1)
      throw new Error(message)
    }
  }
  function result(id, ok) {
    service.receive(JSON.stringify({type: "result", id: id, ok: ok, error: ok ? "" : "Test failure"}))
  }
  function snapshot(volume) {
    service.receive(JSON.stringify({type: "snapshot", status: "ready", selected: "office", volume: volume}))
  }
  Timer {
    interval: 10; repeat: true; running: true
    onTriggered: {
      if (!service.ready) return
      stop()
      var id = service.send("refresh")
      root.check(id !== "", "send returns a request ID")
      root.check(service.busy, "request is pending")
      root.result(id, true)
      root.check(service.busy && root.completions.length === 0, "result alone does not settle request")
      root.snapshot(28)
      root.check(!service.busy, "snapshot clears pending request")
      root.check(root.completions.length === 1 && root.completions[0].id === id, "completion is correlated")
      root.check(root.completions[0].volume === 28, "completion observes refreshed snapshot")
      service.volume.setVolume(80)
      firstVolume.restart()
    }
  }
  Timer {
    id: firstVolume
    interval: 160
    onTriggered: {
      var id = service.volume.requestId
      root.check(id !== "" && service.busy, "volume request started")
      var other = service.send("rpLogin")
      root.result(other, false)
      root.snapshot(28)
      root.check(service.busy, "unrelated completion preserves pending volume request")
      root.check(service.volume.displayValue === 80, "unrelated error preserves volume preview")
      root.result(id, true)
      root.check(service.volume.displayValue === 80, "volume result waits for refreshed state")
      root.snapshot(70)
      root.check(service.volume.displayValue === 70, "speaker cap replaces preview")
      root.check(!service.busy && service.volume.requestId === "", "volume request settled")
      var counter = service.requestCounter
      service.volume.setVolume(70)
      root.check(service.volume.previewValue === -1 && service.requestCounter === counter, "no-op volume needs no request")
      service.volume.setVolume(80)
      failedVolume.restart()
    }
  }
  Timer {
    id: failedVolume
    interval: 160
    onTriggered: {
      var id = service.volume.requestId
      root.check(id !== "", "second volume request started")
      root.result(id, false)
      root.snapshot(70)
      root.check(service.volume.displayValue === 70 && service.volume.previewValue === -1, "own failure clears preview")
      root.snapshot(62)
      root.check(service.volume.displayValue === 62, "external volume updates when idle")
      console.log("SERVICE TESTS PASSED")
      Qt.quit()
    }
  }
  Timer { interval: 5000; running: true; onTriggered: { console.error("SERVICE TEST TIMEOUT"); Qt.exit(1) } }
}
