Git is read-only while closing: `status`, `diff`, `log` and `show` only.

Closing means every in-scope objective is finished with full confidence, and the close carries no hedges: no "not verified", "should work" or caveat section. What the Objective excludes or was decided to leave for later is not mentioned unless a step asks for it. A passing idea or a drift is ignored.

If the task is not closable, stop and say "I can't close this because <the objective and the concrete evidence>", and change nothing. Refusing beats closing with doubt.

Evidence already gathered counts: checks that passed after the last code change are not rerun, and files read in this session are not reread. Skip a step with nothing to act on and leave it out of the report.
