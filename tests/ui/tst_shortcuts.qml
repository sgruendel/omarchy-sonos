import QtQuick
import QtQuick.Controls
import QtTest
import "../.." as Sonos

Item {
  width: 500; height: 350
  property int playCount: 0
  property int skipCount: 0
  property int volumeDelta: 0
  property int muteCount: 0
  property int buttonCount: 0
  property int mixSelections: 0
  property real sliderValue: 28
  Sonos.PlayerShortcuts {
    id: shortcuts
    active: true
    editing: username.activeFocus || password.activeFocus
    spaceReserved: mix.activeFocus || mix.popup.visible
    onTogglePlayback: playCount++
    onNext: skipCount++
    onPrevious: skipCount--
    onAdjustVolume: function(delta) { volumeDelta += delta }
    onToggleMute: muteCount++
  }
  Sonos.TransportButton { id: nextButton; text: "Next"; onClicked: buttonCount++ }
  Sonos.VolumeSlider { id: slider; y: 50; width: 300; sourceValue: sliderValue; onCommitted: function(value) { sliderValue = value } }
  ComboBox { id: mix; y: 100; model: ["Main", "Mellow", "Rock"]; onActivated: mixSelections++ }
  TextField { id: username; y: 150; width: 300 }
  TextField { id: password; y: 200; width: 300; echoMode: TextInput.Password }
  TestCase {
    name: "PlayerShortcutRouting"
    when: windowShown
    function init() {
      mix.popup.close()
      tryCompare(mix.popup, "visible", false)
      shortcuts.active = true
      playCount = 0; skipCount = 0; volumeDelta = 0; muteCount = 0; buttonCount = 0
      mixSelections = 0
      sliderValue = 28; mix.currentIndex = 0; username.text = ""; password.text = ""
      nextButton.forceActiveFocus()
    }
    function test_space_on_next_button_pauses() {
      keyClick(Qt.Key_Space)
      compare(playCount, 1)
      compare(buttonCount, 0)
    }
    function test_ctrl_arrows_on_slider_use_player_shortcuts() {
      slider.forceActiveFocus()
      keyClick(Qt.Key_Up, Qt.ControlModifier)
      compare(volumeDelta, 5)
      compare(sliderValue, 28)
      keyClick(Qt.Key_Right, Qt.ControlModifier)
      compare(skipCount, 1)
      compare(sliderValue, 28)
      keyClick(Qt.Key_Right)
      compare(sliderValue, 33)
    }
    function test_ctrl_arrows_do_not_change_mix() {
      mix.forceActiveFocus()
      keyClick(Qt.Key_Down, Qt.ControlModifier)
      compare(volumeDelta, -5)
      compare(mix.currentIndex, 0)
    }
    function test_space_opens_mix_and_enter_confirms_selection() {
      mix.forceActiveFocus()
      keyClick(Qt.Key_Space)
      tryCompare(mix.popup, "visible", true)
      compare(playCount, 0)
      compare(mixSelections, 0)
      keyClick(Qt.Key_Down)
      compare(mix.currentIndex, 0)
      compare(mixSelections, 0)
      keyClick(Qt.Key_Return)
      tryCompare(mix.popup, "visible", false)
      compare(mix.currentIndex, 1)
      compare(mixSelections, 1)
      compare(playCount, 0)
    }
    function test_space_in_open_mix_does_not_toggle_playback() {
      mix.forceActiveFocus()
      keyClick(Qt.Key_Space)
      tryCompare(mix.popup, "visible", true)
      keyClick(Qt.Key_Space)
      compare(playCount, 0)
    }
    function test_enter_still_activates_buttons() {
      keyClick(Qt.Key_Return)
      compare(buttonCount, 1)
      compare(playCount, 0)
    }
    function test_username_editing_is_uninterrupted() {
      username.forceActiveFocus()
      keyClick(Qt.Key_M)
      keyClick(Qt.Key_Space)
      keyClick(Qt.Key_Right, Qt.ControlModifier)
      compare(username.text, "m ")
      compare(playCount, 0); compare(muteCount, 0); compare(skipCount, 0)
    }
    function test_password_editing_is_uninterrupted() {
      password.forceActiveFocus()
      keyClick(Qt.Key_M)
      keyClick(Qt.Key_Space)
      compare(password.text, "m ")
      compare(playCount, 0); compare(muteCount, 0)
    }
    function test_closed_popup_disables_shortcuts() {
      shortcuts.active = false
      keyClick(Qt.Key_Space)
      compare(playCount, 0)
      compare(buttonCount, 1)
    }
  }
}
