import QtQuick
import QtTest
import "../.." as Sonos

Item {
  width: 500; height: 200
  QtObject {
    id: service
    property bool ready: true
    property bool busy: false
    property string lastError: ""
    property var snapshot: ({selected: "office", volume: 28})
    property var requests: []
    property int counter: 0
    signal commandFinished(string requestId, bool ok)
    function send(op, args) {
      var id = String(++counter)
      requests = requests.concat([{id: id, op: op, volume: args.volume}])
      busy = true
      return id
    }
    function finish(id, volume, ok) {
      snapshot = {selected: snapshot.selected, volume: volume}
      busy = false
      commandFinished(id, ok === undefined ? true : ok)
    }
  }
  Sonos.VolumeController { id: controller; service: service }
  Sonos.VolumeSlider {
    id: slider
    width: 300
    sourceValue: controller.displayValue
    contextKey: controller.contextKey
    enabled: service.ready
    onCommitted: function(value) { controller.setVolume(value) }
  }
  Sonos.VolumeSlider {
    id: secondSlider
    y: 60; width: 300
    sourceValue: controller.displayValue
    contextKey: controller.contextKey
    enabled: service.ready
    onCommitted: function(value) { controller.setVolume(value) }
  }
  TestCase {
    name: "VolumeInteraction"
    when: windowShown
    function init() {
      controller.reset()
      service.ready = true
      service.busy = false
      service.snapshot = {selected: "office", volume: 28}
      service.requests = []
      service.lastError = ""
      slider.forceActiveFocus()
    }
    function test_repeated_input_stays_focused_and_coalesces() {
      keyClick(Qt.Key_Right)
      keyClick(Qt.Key_Right)
      compare(slider.value, 38)
      compare(secondSlider.value, 38)
      compare(service.requests.length, 0)
      tryVerify(function() { return service.requests.length === 1 })
      compare(service.requests[0].volume, 38)
      verify(service.busy)
      verify(slider.enabled && slider.activeFocus)
      keyClick(Qt.Key_Right)
      compare(slider.value, 43)
      wait(160)
      compare(service.requests.length, 1)
      service.finish(service.requests[0].id, 38)
      compare(service.requests.length, 2)
      compare(service.requests[1].volume, 43)
      service.finish(service.requests[1].id, 40) // speaker caps volume
      compare(slider.value, 40)
      compare(controller.previewValue, -1)
    }
    function test_snapshot_can_keep_the_same_value() {
      controller.setVolume(80)
      tryVerify(function() { return service.requests.length === 1 })
      service.finish(service.requests[0].id, 28)
      compare(slider.value, 28)
      compare(controller.previewValue, -1)
    }
    function test_return_to_current_volume_needs_no_request() {
      keyClick(Qt.Key_Right)
      keyClick(Qt.Key_Left)
      compare(slider.value, 28)
      compare(controller.previewValue, -1)
      wait(160)
      compare(service.requests.length, 0)
    }
    function test_errors_are_correlated() {
      controller.setVolume(55)
      tryVerify(function() { return service.requests.length === 1 })
      service.lastError = "Unrelated account failure"
      service.commandFinished("unrelated", false)
      compare(controller.displayValue, 55)
      service.finish(service.requests[0].id, 28, false)
      compare(slider.value, 28)
      compare(controller.previewValue, -1)
    }
    function test_input_waits_for_unrelated_command() {
      service.busy = true
      controller.setVolume(55)
      wait(160)
      compare(service.requests.length, 0)
      compare(slider.value, 55)
      verify(slider.enabled && slider.activeFocus)
      service.busy = false
      compare(service.requests.length, 1)
      compare(service.requests[0].volume, 55)
    }
    function test_room_switch_discards_queued_input() {
      controller.setVolume(55)
      tryVerify(function() { return service.requests.length === 1 })
      var oldId = service.requests[0].id
      controller.setVolume(60)
      service.snapshot = {selected: "living", volume: 17}
      service.finish(oldId, 17)
      wait(160)
      compare(service.requests.length, 1)
      compare(slider.value, 17)
      compare(controller.requestId, "")
    }
    function test_external_volume_updates_when_idle() {
      service.snapshot = {selected: "office", volume: 62}
      compare(slider.value, 62)
    }
    function test_drag_sends_only_final_value() {
      mousePress(slider, 80, 16)
      mouseMove(slider, 220, 16)
      compare(service.requests.length, 0)
      mouseRelease(slider, 220, 16)
      tryVerify(function() { return service.requests.length === 1 })
      compare(service.requests[0].volume, Math.round(slider.value))
      verify(service.requests[0].volume > 70)
    }
    function test_track_click_uses_final_value() {
      mouseClick(slider, 220, 16)
      tryVerify(function() { return service.requests.length === 1 })
      compare(service.requests[0].volume, Math.round(slider.value))
      verify(service.requests[0].volume > 70)
    }
    function test_room_switch_during_drag() {
      mousePress(slider, 80, 16)
      mouseMove(slider, 220, 16)
      service.snapshot = {selected: "living", volume: 17}
      mouseRelease(slider, 220, 16)
      tryCompare(slider, "value", 17)
      wait(160)
      compare(service.requests.length, 0)
    }
    function test_disconnect_cancels_input() {
      controller.setVolume(55)
      service.ready = false
      wait(160)
      compare(service.requests.length, 0)
      compare(controller.previewValue, -1)
      verify(!slider.enabled)
    }
    function test_wheel_steps_by_five() {
      mouseWheel(slider, 150, 16, 0, 120)
      compare(slider.value, 33)
      tryVerify(function() { return service.requests.length === 1 })
      compare(service.requests[0].volume, 33)
    }
  }
}
