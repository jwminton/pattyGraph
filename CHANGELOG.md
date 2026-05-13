# Changelog

## 0.1.1

- Added a focused Go test suite covering parser behavior, buffers, tokenization, matcher behavior, monitor pipeline flow, inline commands, config/output handling, and control-file gating.
- Added optional `pattyControl.log` command input support, with CLI/inline controls for enabling and disabling processing from the control file.
- Allowed nginx access log lines with a trailing `"-"` field after the User-Agent, while preserving the standard User-Agent token space.
- Hardened User-Agent parsing so trailing whitespace and the supported trailing field do not pollute User-Agent tokenization.
