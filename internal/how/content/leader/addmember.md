# Adding thinkers to your live workgroup

Add a thinker when the work needs an area or a view that no current thinker owns, such as a second reviewer or someone for a part of the code nobody holds. Do not add one to speed up a thinker who already owns the work, since thinkers share one working tree and two in the same files collide.

1. Write a config that lists only the new thinkers. The group's name and your leader label come from the live group, so the config must not carry `name` or `leader_label`. Keep it in your task directory, for example `biblio/scratchpad/<task-slug>/add-docs.yaml`:

   ```yaml
   thinkers:
     - label: docs
       vendor: claude
       skills: [lgrass-connector]
       prompt: |
         Own the user docs for the new export command. The CLI shape is settled in the group thread.
   ```

   Each thinker follows the same rules as at create: a label of letters, digits, `-` and `_` up to 30 characters that no member of the group already uses, the vendor `claude` or `codex`, every skill installed for that vendor, and a prompt (or `prompt_file`) of up to 3000 characters.

2. Run `lgrass workgroup add <path-to-config>` from your tab. It checks the config, then waits while the human reads the new thinkers' full starting text in the app. A decline prints the human's reason when there is one, so revise and propose again. A withdrawal means nobody answered, so ask the human before trying again.

3. On approval the new tabs open beside yours and lgrass posts a join message to the group thread, so every member knows who arrived. Each new thinker is told the group was already working, reads the whole thread, loads its skills and posts its plan.

4. Brief the new thinker in the group thread with a mention: what it owns, which files are its own, and what the group has already decided that its assignment does not say. Then answer its plan with go or with what to change, the same as any thinker.
