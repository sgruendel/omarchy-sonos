pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls as Controls
import Quickshell
import qs.Ui
import qs.Commons

BarWidget {
  id: root
  moduleName: "sgruendel.sonos"
  readonly property var service: bar?.shell?.serviceFor(moduleName)
  readonly property var snap: service ? service.snapshot : ({})
  readonly property var playback: snap.playback || ({})
  readonly property var radio: snap.rp || ({})
  readonly property var song: radio.song || null
  readonly property var account: snap.account || ({})
  readonly property var rooms: snap.rooms || []
  readonly property bool ready: !!service && service.ready
  readonly property bool busy: !!service && service.busy
  readonly property bool playing: playback.state === "PLAYING"
  readonly property string cover: song ? song.cover : (playback.artwork || "")
  property bool popupOpen: false
  property bool accountOpen: false
  readonly property bool opened: popupOpen
  readonly property var mixes: snap.rpMixes || [{id:-1,name:"Automatic"}]
  function open() { popupOpen = true }
  function close() { popupOpen = false; password.text = "" }
  function toggle() { if (popupOpen) close(); else open() }
  function send(op, args) { if (service) service.send(op, args) }
  function hasAction(action) { return (playback.actions || []).indexOf(action) >= 0 }
  function signIn() {
    var value = password.text
    password.text = ""
    send("rpLogin", {username: username.text, password: value})
  }
  implicitWidth: barButton.implicitWidth
  implicitHeight: barButton.implicitHeight

  WidgetButton {
    id: barButton
    bar: root.bar
    text: root.vertical ? "♪" : "♪ " + (root.ready ? (root.song ? root.song.title : (root.playback.title || "Sonos")) : "Sonos")
    fixedWidth: root.vertical ? root.barSize : Math.min(barText.implicitWidth + Style.space(20), Number(root.setting("maxLabelWidth",220)))
    labelVisible: false
    tooltipText: root.ready ? (root.playback.artist || "Sonos") + " · " + (root.playback.title || root.playback.state) : "Open Sonos Paradise"
    Text {
      id: barText
      anchors.fill: parent
      anchors.margins: Style.space(8)
      text: barButton.text
      textFormat: Text.PlainText
      color: Color.foreground
      font.family: Style.font.family
      font.pixelSize: Style.font.body
      elide: Text.ElideRight
      verticalAlignment: Text.AlignVCenter
      horizontalAlignment: Text.AlignHCenter
    }
    onPressed: function(button) {
      if (button === Qt.MiddleButton && root.ready && !root.busy) root.send("playPause")
      else if (button === Qt.LeftButton) root.toggle()
    }
    onWheelMoved: function(delta) { if (root.ready && !root.busy) root.send("adjustVolume", {delta: delta > 0 ? 5 : -5}) }
  }

  component DetailText: Text {
    width: parent.width
    textFormat: Text.PlainText
    wrapMode: Text.Wrap
    color: Color.foreground
    font.family: Style.font.family
    font.pixelSize: Style.font.body
  }

  KeyboardPanel {
    id: popup
    anchorItem: barButton
    bar: root.bar
    owner: root
    open: root.popupOpen
    focusTarget: panelContent
    contentWidth: fittedContentWidth(Style.space(400))
    contentHeight: fittedContentHeight(content.implicitHeight, Style.space(700))

    FocusScope {
      id: panelContent
      anchors.fill: parent
      Keys.onEscapePressed: root.close()
      Flickable {
        id: scroll
        anchors.fill: parent
        contentWidth: width
        contentHeight: content.implicitHeight
        clip: true
        boundsBehavior: Flickable.StopAtBounds
        Controls.ScrollBar.vertical: Controls.ScrollBar {}
        Column {
          id: content
          width: scroll.width
          spacing: Style.space(12)
          DetailText { text: "Sonos Paradise"; font.bold: true; font.pixelSize: Style.font.body * 1.25 }
          DetailText { visible: !root.ready; text: root.snap.error || (root.snap.status === "starting" ? "Looking for speakers…" : "No reachable Sonos speakers") }
          DetailText { visible: text !== ""; text: (root.service ? root.service.lastError : "") || (root.ready ? root.snap.error : "") || ""; color: Color.accent }
          DetailText { visible: text !== ""; text: root.service ? root.service.lastDiagnostic : ""; color: Color.accent }
          Row {
            spacing: Style.space(8)
            ActionButton { text: "Refresh"; enabled: !root.busy; onClicked: root.send("refresh") }
            ActionButton { text: root.account.authenticated ? "RP: " + root.account.username : "RP account"; onClicked: root.accountOpen = !root.accountOpen }
          }
          Column {
            width: parent.width
            spacing: Style.space(8)
            visible: root.accountOpen
            DetailText { text: root.account.authenticated ? "Signed in as " + root.account.username : "Sign in to Radio Paradise to rate the song playing on Sonos." }
            Controls.TextField {
              id: username
              width: parent.width
              visible: !root.account.authenticated
              placeholderText: "RP username"
              color: Color.foreground
              palette.text: Color.foreground
              palette.base: Color.background
              selectByMouse: true
            }
            Controls.TextField {
              id: password
              width: parent.width
              visible: !root.account.authenticated
              placeholderText: "RP password"
              echoMode: TextInput.Password
              color: Color.foreground
              palette.text: Color.foreground
              palette.base: Color.background
              onAccepted: if (!root.busy) root.signIn()
            }
            ActionButton {
              text: root.account.authenticated ? "Sign out" : "Sign in"
              enabled: !root.busy && (root.account.authenticated || (username.text.length > 0 && password.text.length > 0))
              onClicked: { if (root.account.authenticated) root.send("rpLogout"); else root.signIn() }
            }
          }
          DetailText { visible: root.rooms.length > 0; text: "Control room · volume affects this room"; opacity: 0.65 }
          Flow {
            width: parent.width
            spacing: Style.space(6)
            Repeater {
              model: root.rooms
              ActionButton {
                required property var modelData
                text: modelData.name
                selected: root.snap.selected === modelData.uid
                enabled: !root.busy
                onClicked: root.send("selectRoom", {room: modelData.uid})
              }
            }
          }
          Image {
            width: parent.width
            height: Math.min(parent.width, Style.space(260))
            source: /^https?:\/\//.test(root.cover) ? root.cover : ""
            visible: status === Image.Ready
            asynchronous: true
            fillMode: Image.PreserveAspectFit
          }
          Column {
            width: parent.width
            spacing: Style.space(4)
            visible: root.ready
            DetailText { text: root.song ? root.song.title : (root.playback.title || "Nothing playing"); font.bold: true }
            DetailText { text: root.song ? root.song.artist : (root.playback.artist || root.playback.station || ""); opacity: 0.8 }
            DetailText { text: root.song ? root.song.album + (root.song.year ? " · " + root.song.year : "") : (root.playback.album || ""); visible: text !== ""; opacity: 0.6 }
          }
          Row {
            spacing: Style.space(8)
            visible: root.ready
            ActionButton { text: "Previous"; enabled: !root.busy && root.hasAction("Previous"); onClicked: root.send("previous") }
            ActionButton { text: root.playing ? "Pause" : "Play"; enabled: !root.busy && root.hasAction(root.playing ? "Pause" : "Play"); onClicked: root.send("playPause") }
            ActionButton { text: "Next"; enabled: !root.busy && root.hasAction("Next"); onClicked: root.send("next") }
          }
          Row {
            spacing: Style.space(8)
            visible: root.ready
            ActionButton { text: "−"; enabled: !root.busy; onClicked: root.send("adjustVolume", {delta:-5}) }
            ActionButton { text: String(root.snap.volume || 0) + "%"; enabled: false }
            ActionButton { text: "+"; enabled: !root.busy; onClicked: root.send("adjustVolume", {delta:5}) }
            ActionButton { text: root.snap.mute ? "Unmute" : "Mute"; enabled: !root.busy; onClicked: root.send("setMute", {mute:!root.snap.mute}) }
          }
          Column {
            width: parent.width
            spacing: Style.space(10)
            visible: root.ready
            DetailText { text: root.radio.detected ? "Radio Paradise · " + (root.radio.channelName || "Unknown mix") : "RP mix override"; font.bold: true }
            Controls.ComboBox {
              width: parent.width
              model: root.mixes
              textRole: "name"
              valueRole: "id"
              enabled: !root.busy
              currentIndex: { for (var i=0; i<root.mixes.length; ++i) if (root.mixes[i].id === root.radio.override) return i; return 0 }
              onActivated: function(index) { root.send("rpChannel", {channel: root.mixes[index].id}) }
              palette.text: Color.foreground
              palette.buttonText: Color.foreground
              palette.button: Color.background
              palette.base: Color.background
            }
            DetailText { visible: !!root.radio.error; text: root.radio.error || ""; opacity: 0.7 }
            DetailText {
              visible: !root.radio.detected && root.account.authenticated
              text: "No RP song identified. If you’re listening to Radio Paradise, select the matching mix above to enable ratings."
              opacity: 0.7
            }
            DetailText { visible: !!root.song; text: root.song ? "Listeners: " + root.song.listenerRating.toFixed(1) + "/10 · " + root.song.ratingsCount + " ratings" : "" }
            DetailText { visible: !!root.song; text: root.account.authenticated ? "Your rating: " + (root.song && root.song.userRating > 0 ? root.song.userRating + "/10" : "Not rated") : "Sign in through RP account to rate this song." }
            Flow {
              width: parent.width
              spacing: Style.space(4)
              visible: !!root.song && root.account.authenticated
              Repeater {
                model: 10
                ActionButton {
                  required property int index
                  text: String(index + 1)
                  selected: !!root.song && root.song.userRating === index + 1
                  enabled: !root.busy && root.radio.canRate === true
                  onClicked: root.send("rpRate", {songId: root.song.id, room: root.snap.selected, rating: index + 1})
                }
              }
            }
            DetailText { visible: !!root.song; text: "Comments · " + ((root.radio.comments || {}).total || 0); font.bold: true }
            DetailText { visible: !!root.radio.commentsError; text: root.radio.commentsError || ""; color: Color.accent }
            Repeater {
              model: (root.radio.comments || {}).items || []
              Column {
                required property var modelData
                width: content.width
                spacing: Style.space(4)
                DetailText { text: parent.modelData.username + " · " + parent.modelData.posted; opacity: 0.65; font.pixelSize: Style.font.body * 0.9 }
                DetailText { text: parent.modelData.message }
                DetailText { text: "+" + parent.modelData.upvotes + " / −" + parent.modelData.downvotes; opacity: 0.5; font.pixelSize: Style.font.body * 0.85 }
                Rectangle { width: parent.width; height: 1; color: Color.foreground; opacity: 0.1 }
              }
            }
            ActionButton {
              visible: !!root.song
              text: !root.account.authenticated ? "Sign in for comments" : (root.radio.commentsError ? "Retry comments" : "More comments")
              enabled: !root.busy && (!root.account.authenticated || !!root.radio.commentsError || (root.radio.comments || {}).more === true)
              onClicked: {
                if (!root.account.authenticated) root.accountOpen = true
                else root.send("rpComments", {songId: root.song.id, offset: root.radio.commentsError ? 0 : root.radio.comments.offset})
              }
            }
          }
        }
      }
    }
  }
}
