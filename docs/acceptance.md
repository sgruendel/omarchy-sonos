# Live acceptance checks

These checks require an Omarchy session, reachable Sonos, and your RP account.
The automated suite does not certify speaker firmware or account API behavior.

1. Build/install, open the widget, verify discovered rooms and a grouped room's
   coordinator playback. Confirm volume changes affect only the selected room.
2. Verify available playback actions on local tracks and internet radio. Verify
   disabled Next/Previous on sources that cannot skip.
3. Play RP Main Mix on Sonos; compare the popup to what you hear, including
   artwork, album, listener rating, and comments.
4. Try another RP mix and an opaque TuneIn URL. Use its mix override if automatic
   detection cannot identify it. Switch back to Automatic afterward.
5. Sign in through the popup. Submit a rating, check it on RP, restart the shell,
   and verify the session/personal rating survives. Sign out and verify the session
   file disappears. Wrong credentials should produce a readable error.
6. Load another comments page and check that pages append. Switch songs and
   confirm comments reset to the newly matched song.
7. Pause or switch songs while the popup is open. An outdated rating command
   should fail instead of rating a different song.
8. Switch to a non-RP source in Automatic mode and verify RP details disappear.
   Disconnect/reconnect the LAN and check offline state and recovery.
9. Open the widget on two monitors and confirm one backend/session is shared.
   Check keyboard Tab, Escape, outside-click dismissal, and a narrow screen.
10. Drag the volume slider and release: its percentage should hold until Sonos
    confirms the change. Test keyboard arrows, wheel, mute/unmute, command failure,
    and switching rooms; a previous room's pending volume must not carry over.
    Hold an arrow key and scroll repeatedly during slow requests: focus should
    stay on the slider and only the latest volume should be queued. Check a
    speaker volume limit, returning to the current value, and volume changes
    from speaker buttons. An unrelated RP failure must not reset the preview.
11. Verify Space, Ctrl+Left/Right, Ctrl+Up/Down, and M with the popup focused.
    Repeat with Next, the volume slider, and the mix dropdown focused:
    Ctrl+Up/Down adjusts volume by 5 and the mix stays selected. Space toggles
    playback from Next or the slider. On the focused mix dropdown, Space opens
    the list; Up/Down browses options and Enter confirms once without toggling
    playback or sending a request for each arrow press.
    Type a username/password containing spaces and M without triggering playback
    or mute. Verify timed tracks show progress; live radio has no invented duration.
12. Expand room selection and Radio Paradise details. Check rating selection,
    comments pagination, account access, long track/room names, and missing artwork.
    Confirm the header shows the RP username when signed in and Signed out after
    logout, without needing a mouse hover.
