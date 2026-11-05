; SPDX-License-Identifier: AGPL-3.0-only
; Copyright (C) 2026 The Lemongrass Authors

; Runs before the uninstaller removes the files, so the bundled lgrassconf.exe is still there. It stops the keeper and
; removes its logon task, the PATH entry, the hooks and skills and the copied binaries; the lemongrass home stays. An
; upgrade runs the old uninstaller too, and then nothing is removed, since the new version's first launch would only
; put it all back.
!macro customUnInstall
  ${ifNot} ${isUpdated}
    nsExec::Exec '"$INSTDIR\resources\bin\windows-amd64\lgrassconf.exe" uninstall'
    Pop $0
  ${endIf}
!macroend
