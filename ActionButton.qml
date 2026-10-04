import QtQuick
import QtQuick.Controls
import qs.Commons

Button {
  id: root
  property bool selected: false
  implicitHeight: Style.space(32)
  implicitWidth: Math.max(Style.space(30), label.implicitWidth + Style.space(20))
  leftPadding: Style.space(10)
  rightPadding: Style.space(10)
  opacity: enabled ? 1 : 0.4
  contentItem: Text {
    id: label
    text: root.text
    textFormat: Text.PlainText
    color: root.selected ? Color.background : Color.foreground
    font.family: Style.font.family
    font.pixelSize: Style.font.body
    horizontalAlignment: Text.AlignHCenter
    verticalAlignment: Text.AlignVCenter
    elide: Text.ElideRight
  }
  background: Rectangle {
    radius: Style.space(4)
    color: root.selected ? Color.accent : (root.down ? Qt.rgba(1,1,1,0.16) : Qt.rgba(1,1,1,0.06))
    border.width: root.activeFocus ? 2 : 1
    border.color: root.activeFocus ? Color.accent : Qt.rgba(1,1,1,0.15)
  }
}
