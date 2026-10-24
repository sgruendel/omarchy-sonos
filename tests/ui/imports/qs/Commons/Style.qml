pragma Singleton
import QtQuick
// Only theme tokens are substituted; tests use the real Qt controls and the
// production Sonos components without requiring Quickshell's embedded plugins.
QtObject {
  property QtObject font: QtObject {
    property string family: "monospace"
    property int body: 12
    property int icon: 16
    property int iconLarge: 20
  }
  function space(value) { return value }
  function selectedStateColor(foreground, accent) { return foreground }
  function selectedFillFor(foreground, accent) { return "transparent" }
  function pressedFillFor(foreground, accent) { return "transparent" }
  function hoverFillFor(foreground, accent) { return "transparent" }
}
