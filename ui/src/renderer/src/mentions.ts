export interface BodyPart {
  text: string
  mention: boolean
}

// The closing may be <<!, <<, <!, < or absent, matching what lgrass accepts as a mention.
const MENTION = /!>>([0-9a-f-]{8,36})(?:<<?!?)?/gi

// Mentions in a message body are tab ids or their first 8 characters, shown as the member's label.
export function bodyParts(body: string, members: { tabId: string; label: string }[]): BodyPart[] {
  const parts: BodyPart[] = []
  let last = 0
  for (const match of body.matchAll(MENTION)) {
    const id = match[1].toLowerCase()
    const member = members.find((m) => m.tabId.toLowerCase().startsWith(id))
    if (match.index > last) parts.push({ text: body.slice(last, match.index), mention: false })
    parts.push({ text: member ? `@${member.label}` : match[0], mention: !!member })
    last = match.index + match[0].length
  }
  if (last < body.length) parts.push({ text: body.slice(last), mention: false })
  return parts
}
