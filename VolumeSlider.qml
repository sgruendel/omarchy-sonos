import QtQuick
import QtQuick.Controls
import qs.Commons
import qs.Commons as Commons

Slider {
  id: root
  property real sourceValue: 0
  property string contextKey: ""
  property string dragContext: ""
  signal committed(int volume)

  from: 0
  to: 100
  stepSize: 5
  wheelEnabled: true
  implicitHeight: Style.space(32)
  leftPadding: Style.space(7)
  rightPadding: Style.space(7)
  Accessible.name: "Room volume"

  function synchronize() {
    if (!pressed) value = sourceValue
  }
  function commit() {
    committed(Math.round(value))
  }
  onPressedChanged: {
    if (pressed) dragContext = contextKey
    else if (dragContext === contextKey && enabled) commit()
    // Restore authoritative state after a cancelled drag or a no-op release.
    if (!pressed) Qt.callLater(synchronize)
  }
  // Keyboard and wheel changes commit once; dragging commits on release.
  onMoved: if (!pressed) commit()
  onSourceValueChanged: synchronize()
  onContextKeyChanged: synchronize()
  onEnabledChanged: if (!enabled) dragContext = ""
  Component.onCompleted: synchronize()

  background: Rectangle {
    x: root.leftPadding
    y: (root.height - height) / 2
    width: root.availableWidth
    height: Style.space(4)
    radius: height / 2
    color: Qt.alpha(Commons.Color.foreground, 0.15)
    Rectangle {
      width: parent.width * root.visualPosition
      height: parent.height
      radius: parent.radius
      color: Commons.Color.foreground
      opacity: root.enabled ? 1 : 0.4
    }
  }
  handle: Rectangle {
    x: root.leftPadding + root.visualPosition * (root.availableWidth - width)
    y: (root.height - height) / 2
    width: Style.space(root.pressed || root.hovered ? 14 : 12)
    height: width
    radius: width / 2
    color: Commons.Color.foreground
    opacity: root.enabled ? 1 : 0.4
    border.width: root.activeFocus ? 2 : 0
    border.color: Commons.Color.accent
    Behavior on width { NumberAnimation { duration: 100 } }
  }
}
