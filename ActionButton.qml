import QtQuick
import QtQuick.Controls
import qs.Commons
import qs.Commons as Commons

Button {
  id: root
  property bool selected: false
  property string tooltipText: ""
  hoverEnabled: true
  focusPolicy: Qt.StrongFocus
  Keys.onReturnPressed: if (enabled) clicked()
  Keys.onEnterPressed: if (enabled) clicked()
  Accessible.name: tooltipText || text
  ToolTip.visible: hovered && tooltipText !== ""
  ToolTip.text: tooltipText
  ToolTip.delay: 500
  implicitHeight: Style.space(32)
  implicitWidth: Math.max(Style.space(30), label.implicitWidth + Style.space(20))
  leftPadding: Style.space(10)
  rightPadding: Style.space(10)
  opacity: enabled ? 1 : 0.4
  contentItem: Text {
    id: label
    text: root.text
    textFormat: Text.PlainText
    color: root.selected ? Style.selectedStateColor(Commons.Color.foreground, Commons.Color.accent) : Commons.Color.foreground
    font.family: Style.font.family
    font.pixelSize: Style.font.body
    horizontalAlignment: Text.AlignHCenter
    verticalAlignment: Text.AlignVCenter
    elide: Text.ElideRight
  }
  background: Rectangle {
    radius: Style.space(4)
    color: root.down ? Style.pressedFillFor(Commons.Color.foreground, Commons.Color.accent)
      : root.hovered ? Style.hoverFillFor(Commons.Color.foreground, Commons.Color.accent)
      : root.selected ? Style.selectedFillFor(Commons.Color.foreground, Commons.Color.accent)
      : root.flat ? "transparent" : Qt.alpha(Commons.Color.foreground, 0.04)
    border.width: root.activeFocus ? 2 : 1
    border.color: root.activeFocus ? Commons.Color.accent : root.flat ? "transparent" : Qt.alpha(Commons.Color.foreground, 0.12)
  }
}
