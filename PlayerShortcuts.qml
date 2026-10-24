import QtQuick

Item {
  id: root
  property bool active: false
  property bool editing: false
  property bool spaceReserved: false
  signal togglePlayback()
  signal previous()
  signal next()
  signal adjustVolume(int delta)
  signal toggleMute()

  // Window shortcuts run before focused buttons, sliders and combo boxes.
  // Leave text editing keys alone and reserve Space for an active mix selector.
  Shortcut { sequence: "Space"; enabled: root.active && !root.editing && !root.spaceReserved; autoRepeat: false; onActivated: root.togglePlayback() }
  Shortcut { sequence: "Ctrl+Left"; enabled: root.active && !root.editing; autoRepeat: false; onActivated: root.previous() }
  Shortcut { sequence: "Ctrl+Right"; enabled: root.active && !root.editing; autoRepeat: false; onActivated: root.next() }
  Shortcut { sequence: "Ctrl+Up"; enabled: root.active && !root.editing; onActivated: root.adjustVolume(5) }
  Shortcut { sequence: "Ctrl+Down"; enabled: root.active && !root.editing; onActivated: root.adjustVolume(-5) }
  Shortcut { sequence: "M"; enabled: root.active && !root.editing; autoRepeat: false; onActivated: root.toggleMute() }
}
