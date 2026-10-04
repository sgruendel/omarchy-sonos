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
