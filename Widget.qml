pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls as Controls
import Quickshell
import qs.Ui
import qs.Commons
import qs.Commons as Commons

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
  property bool detailsOpen: false
  property bool roomsOpen: false
  readonly property bool opened: popupOpen
  readonly property var mixes: snap.rpMixes || [{id:-1,name:"Automatic"}]
  function open() { popupOpen = true }
  function close() { popupOpen = false; password.text = "" }
  function toggle() { if (popupOpen) close(); else open() }
  function send(op, args) { return service ? service.send(op, args) : "" }
  function hasAction(action) { return (playback.actions || []).indexOf(action) >= 0 }
  readonly property string roomName: {
    for (var i = 0; i < rooms.length; ++i)
      if (rooms[i].uid === snap.selected) return rooms[i].name
    return "Choose a room"
  }
  function clock(seconds) {
    var total = Math.max(0, Math.floor(Number(seconds) || 0))
    return Math.floor(total / 60) + ":" + (total % 60 < 10 ? "0" : "") + total % 60
  }
  function transport(op, action) {
    if (ready && !busy && hasAction(action)) send(op)
  }
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
      color: Commons.Color.foreground
      font.family: Style.font.family
      font.pixelSize: Style.font.body
      elide: Text.ElideRight
      verticalAlignment: Text.AlignVCenter
      horizontalAlignment: Text.AlignHCenter
    }
    onPressed: function(button) {
      if (button === Qt.MiddleButton) root.transport("playPause", root.playing ? "Pause" : "Play")
      else if (button === Qt.LeftButton) root.toggle()
    }
    onWheelMoved: function(delta) { if (root.ready) root.service.volume.adjust(delta > 0 ? 5 : -5) }
  }

  component DetailText: Text {
    width: parent.width
    textFormat: Text.PlainText
    wrapMode: Text.Wrap
    color: Commons.Color.foreground
    font.family: Style.font.family
    font.pixelSize: Style.font.body
  }

  component Separator: Rectangle {
    width: parent.width
    height: 1
    color: Qt.alpha(Commons.Color.foreground, 0.1)
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
      PlayerShortcuts {
        active: root.popupOpen
        editing: username.activeFocus || password.activeFocus
        spaceReserved: mixSelector.activeFocus || mixSelector.popup.visible
        onTogglePlayback: root.transport("playPause", root.playing ? "Pause" : "Play")
        onPrevious: root.transport("previous", "Previous")
        onNext: root.transport("next", "Next")
        onAdjustVolume: function(delta) { if (root.ready) root.service.volume.adjust(delta) }
        onToggleMute: if (root.ready && !root.busy) root.send("setMute", {mute: !root.snap.mute})
      }
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
          Row {
            width: parent.width
            spacing: Style.space(6)
            Column {
              width: parent.width - refreshButton.width - accountButton.width - parent.spacing * 2
              anchors.verticalCenter: parent.verticalCenter
              spacing: Style.space(3)
              DetailText { text: "Sonos Paradise"; color: Commons.Color.muted; font.pixelSize: Style.font.caption }
              DetailText {
                text: root.account.authenticated ? "RP: " + root.account.username : "RP: Signed out"
                color: root.account.authenticated ? Commons.Color.accent : Commons.Color.muted
                font.pixelSize: Style.font.caption
                maximumLineCount: 1
                elide: Text.ElideRight
              }
            }
            TransportButton {
              id: refreshButton
              text: "󰑐"
              tooltipText: "Refresh speakers and song details"
              enabled: !root.busy
              onClicked: root.send("refresh")
            }
            TransportButton {
              id: accountButton
              text: "󰀄"
              tooltipText: root.account.authenticated ? "RP: " + root.account.username : "RP account"
              selected: root.accountOpen
              onClicked: root.accountOpen = !root.accountOpen
            }
          }
          DetailText { visible: !root.ready; text: root.snap.error || (root.snap.status === "starting" ? "Looking for speakers…" : "No reachable Sonos speakers") }
          DetailText { visible: text !== ""; text: (root.service ? root.service.lastError : "") || (root.ready ? root.snap.error : "") || ""; color: Commons.Color.accent }
          DetailText { visible: text !== ""; text: root.service ? root.service.lastDiagnostic : ""; color: Commons.Color.accent }
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
              color: Commons.Color.foreground
              palette.text: Commons.Color.foreground
              palette.base: Commons.Color.background
              selectByMouse: true
            }
            Controls.TextField {
              id: password
              width: parent.width
              visible: !root.account.authenticated
              placeholderText: "RP password"
              echoMode: TextInput.Password
              color: Commons.Color.foreground
              palette.text: Commons.Color.foreground
              palette.base: Commons.Color.background
              onAccepted: if (!root.busy) root.signIn()
            }
            ActionButton {
              text: root.account.authenticated ? "Sign out" : "Sign in"
              enabled: !root.busy && (root.account.authenticated || (username.text.length > 0 && password.text.length > 0))
              onClicked: { if (root.account.authenticated) root.send("rpLogout"); else root.signIn() }
            }
          }
          Row {
            id: trackRow
            width: parent.width
            spacing: Style.space(16)
            visible: root.ready
            Rectangle {
              id: artworkFrame
              width: Style.space(88)
              height: width
              radius: Style.space(4)
              color: Qt.alpha(Commons.Color.foreground, 0.06)
              border.width: 1
              border.color: Qt.alpha(Commons.Color.foreground, 0.15)
              clip: true
              Text {
                anchors.centerIn: parent
                text: "󰎈"
                color: Commons.Color.muted
                font.family: Style.font.family
                font.pixelSize: Style.space(32)
                visible: artwork.status !== Image.Ready
              }
              Image {
                id: artwork
                anchors.fill: parent
                anchors.margins: 2
                source: /^https?:\/\//.test(root.cover) ? root.cover : ""
                asynchronous: true
                fillMode: Image.PreserveAspectCrop
              }
            }
            Column {
              width: Math.max(0, trackRow.width - artworkFrame.width - trackRow.spacing)
              anchors.verticalCenter: parent.verticalCenter
              spacing: Style.space(5)
              DetailText {
                text: root.song ? root.song.title : (root.playback.title || "Nothing playing")
                font.bold: true
                font.pixelSize: Style.font.body * 1.15
                maximumLineCount: 2
                elide: Text.ElideRight
              }
              DetailText {
                text: root.song ? root.song.artist : (root.playback.artist || root.playback.station || "")
                visible: text !== ""
                color: Commons.Color.accent
                maximumLineCount: 1
                elide: Text.ElideRight
              }
              DetailText {
                text: root.song ? root.song.album + (root.song.year ? " · " + root.song.year : "") : (root.playback.album || "")
                visible: text !== ""
                color: Commons.Color.muted
                font.pixelSize: Style.font.caption
                maximumLineCount: 1
                elide: Text.ElideRight
              }
            }
          }
          Column {
            width: parent.width
            spacing: Style.space(5)
            visible: root.ready
            // Sonos radio often has no duration. Only show progress for timed tracks.
            Rectangle {
              width: parent.width
              height: Style.space(4)
              radius: height / 2
              color: Qt.alpha(Commons.Color.foreground, 0.15)
              visible: root.playback.duration > 0
              Rectangle {
                width: parent.width * Math.max(0, Math.min(1, (root.playback.position || 0) / Math.max(1, root.playback.duration || 0)))
                height: parent.height
                radius: parent.radius
                color: Commons.Color.foreground
              }
            }
            Row {
              width: parent.width
              visible: root.playback.duration > 0
              DetailText { width: parent.width / 2; text: root.clock(root.playback.position); color: Commons.Color.muted; font.pixelSize: Style.font.caption }
              DetailText { width: parent.width / 2; text: root.clock(root.playback.duration); horizontalAlignment: Text.AlignRight; color: Commons.Color.muted; font.pixelSize: Style.font.caption }
            }
            DetailText {
              visible: !(root.playback.duration > 0)
              text: root.playback.station || (root.radio.detected ? "Radio Paradise · " + (root.radio.channelName || "Live radio") : "")
              height: visible && text !== "" ? implicitHeight : 0
              color: Commons.Color.muted
              font.pixelSize: Style.font.caption
            }
          }
          Row {
            anchors.horizontalCenter: parent.horizontalCenter
            spacing: Style.space(12)
            visible: root.ready
            TransportButton {
              anchors.verticalCenter: parent.verticalCenter
              text: "󰒮"
              tooltipText: "Previous · Ctrl+Left"
              enabled: !root.busy && root.hasAction("Previous")
              onClicked: root.transport("previous", "Previous")
            }
            TransportButton {
              text: root.playing ? "󰏤" : "󰐊"
              tooltipText: (root.playing ? "Pause" : "Play") + " · Space"
              prominent: true
              selected: root.playing
              enabled: !root.busy && root.hasAction(root.playing ? "Pause" : "Play")
              onClicked: root.transport("playPause", root.playing ? "Pause" : "Play")
            }
            TransportButton {
              anchors.verticalCenter: parent.verticalCenter
              text: "󰒭"
              tooltipText: "Next · Ctrl+Right"
              enabled: !root.busy && root.hasAction("Next")
              onClicked: root.transport("next", "Next")
            }
          }
          Row {
            width: parent.width
            spacing: Style.space(8)
            visible: root.ready
            TransportButton {
              id: muteButton
              anchors.verticalCenter: parent.verticalCenter
              text: root.snap.mute || !root.snap.volume ? "󰝟" : "󰕾"
              tooltipText: (root.snap.mute ? "Unmute" : "Mute") + " · M"
              selected: root.snap.mute === true
              enabled: !root.busy
              onClicked: root.send("setMute", {mute: !root.snap.mute})
            }
            VolumeSlider {
              id: volume
              width: Math.max(0, parent.width - muteButton.width - volumeLabel.width - parent.spacing * 2)
              anchors.verticalCenter: parent.verticalCenter
              sourceValue: root.service ? root.service.volume.displayValue : 0
              contextKey: root.snap.selected || ""
              enabled: root.ready
              onCommitted: function(value) { root.service.volume.setVolume(value) }
            }
            DetailText {
              id: volumeLabel
              width: Style.space(38)
              anchors.verticalCenter: parent.verticalCenter
              text: Math.round(volume.value) + "%"
              color: Commons.Color.muted
              font.pixelSize: Style.font.caption
              horizontalAlignment: Text.AlignRight
            }
          }
          Separator {}
          ActionButton {
            width: parent.width
            flat: true
            text: (root.playing ? "Playing on " : "Listening on ") + root.roomName + (root.roomsOpen ? "  󰅃" : "  󰅀")
            tooltipText: "Choose the room to control · volume affects this room only"
            visible: root.rooms.length > 0
            selected: root.roomsOpen
            onClicked: root.roomsOpen = !root.roomsOpen
          }
          Flow {
            width: parent.width
            spacing: Style.space(6)
            visible: root.roomsOpen
            Repeater {
              model: root.rooms
              ActionButton {
                required property var modelData
                width: Math.min(implicitWidth, content.width)
                text: modelData.name
                selected: root.snap.selected === modelData.uid
                enabled: !root.busy
                onClicked: { root.send("selectRoom", {room: modelData.uid}); root.roomsOpen = false }
              }
            }
          }
          ActionButton {
            width: parent.width
            flat: true
            text: "Radio Paradise" + (root.song ? " · " + root.song.listenerRating.toFixed(1) + "/10" : "") + (root.detailsOpen ? "  󰅃" : "  󰅀")
            visible: root.ready
            selected: root.detailsOpen
            tooltipText: "Ratings, comments and RP mix settings"
            onClicked: root.detailsOpen = !root.detailsOpen
          }
          Column {
            width: parent.width
            spacing: Style.space(10)
            visible: root.ready && root.detailsOpen
            DetailText { text: root.radio.detected ? "Radio Paradise · " + (root.radio.channelName || "Unknown mix") : "RP mix override"; font.bold: true }
            Controls.ComboBox {
              id: mixSelector
              width: parent.width
              model: root.mixes
              textRole: "name"
              valueRole: "id"
              enabled: !root.busy
              currentIndex: { for (var i=0; i<root.mixes.length; ++i) if (root.mixes[i].id === root.radio.override) return i; return 0 }
              onActivated: function(index) { root.send("rpChannel", {channel: root.mixes[index].id}) }
              palette.text: Commons.Color.foreground
              palette.buttonText: Commons.Color.foreground
              palette.button: Commons.Color.background
              palette.base: Commons.Color.background
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
            DetailText { visible: !!root.radio.commentsError; text: root.radio.commentsError || ""; color: Commons.Color.accent }
            Repeater {
              model: (root.radio.comments || {}).items || []
              Column {
                required property var modelData
                width: content.width
                spacing: Style.space(4)
                DetailText { text: parent.modelData.username + " · " + parent.modelData.posted; opacity: 0.65; font.pixelSize: Style.font.body * 0.9 }
                DetailText { text: parent.modelData.message }
                DetailText { text: "+" + parent.modelData.upvotes + " / −" + parent.modelData.downvotes; opacity: 0.5; font.pixelSize: Style.font.body * 0.85 }
                Separator {}
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
