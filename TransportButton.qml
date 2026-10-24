import QtQuick
import qs.Commons
import qs.Commons as Commons

ActionButton {
  id: root
  property bool prominent: false
  implicitWidth: Style.space(prominent ? 44 : 36)
  implicitHeight: implicitWidth
  flat: true
  contentItem: Text {
    text: root.text
    color: root.selected ? Style.selectedStateColor(Commons.Color.foreground, Commons.Color.accent) : Commons.Color.foreground
    font.family: Style.font.family
    font.pixelSize: root.prominent ? Style.font.iconLarge : Style.font.icon
    horizontalAlignment: Text.AlignHCenter
    verticalAlignment: Text.AlignVCenter
  }
}
