export type InputKind = 'reply' | 'enter' | 'typing'

// Sequences xterm.js writes on the terminal's own behalf: focus in and out, cursor position and
// device reports, mode reports, OSC and DCS replies, and mouse reports. Everything else on the
// input channel, pastes, arrows and Shift+Enter included, came from the human.
const TERMINAL_REPLIES = new RegExp(
  [
    '\\x1b\\[[IO]',
    '\\x1b\\[\\d+;\\d+R',
    '\\x1b\\[[?>=]?[\\d;]*c',
    '\\x1b\\[\\d*n',
    '\\x1b\\[\\?[\\d;]*\\$y',
    '\\x1b\\][^\\x07\\x1b]*(?:\\x07|\\x1b\\\\)',
    '\\x1bP[^\\x1b]*\\x1b\\\\',
    '\\x1b\\[<\\d+;\\d+;\\d+[Mm]',
    '\\x1b\\[M[\\s\\S]{3}'
  ].join('|'),
  'g'
)

export function classifyInput(data: string): InputKind {
  const human = data.replace(TERMINAL_REPLIES, '')
  if (human === '') return 'reply'
  if (human === '\r') return 'enter'
  return 'typing'
}
