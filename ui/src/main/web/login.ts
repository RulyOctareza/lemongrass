// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { click, selectorTarget, settle, type, waitSelector, waitUrl } from './actions'
import type { Page } from './page'
import { WebError, type WebLoginFields } from './types'

const DEFAULT_WAIT_SECONDS = 10

// The top of the page the login stopped on, so a rejected login shows the page's own message. The snapshot
// already masks password inputs.
async function pageExcerpt(page: Page): Promise<string> {
  const snapshot = await page.snapshot().catch(() => '')
  const lines = snapshot.split('\n').slice(0, 20)
  return lines.length > 1 ? `\nThe page now:\n${lines.join('\n')}` : ''
}

// Runs the rester's web login steps on a fresh page. The values come from the vault op; execute() scrubs
// every text that leaves, so none can reach a result; the result is the user's page left where the steps ended.
export async function runLogin(page: Page, login: WebLoginFields, user: string): Promise<void> {
  const steps = login.login ?? []
  const fields = login.fields ?? {}
  const origin = login.origin
  for (const [index, step] of steps.entries()) {
    const where = `login step ${index + 1} (${step.Action})`
    try {
      if (step.Action === 'goto') {
        await page.goto(new URL(step.URL, origin).toString(), where)
      } else if (step.Action === 'fill') {
        if (!(step.Field in fields))
          throw new WebError(`${where}: ${user} has no field named ${step.Field}`)
        await type(page, selectorTarget(page, step.Selector), fields[step.Field])
      } else if (step.Action === 'click') {
        await click(page, selectorTarget(page, step.Selector))
        await settle(page)
      } else if (step.Action === 'wait') {
        const seconds = step.TimeoutSeconds > 0 ? step.TimeoutSeconds : DEFAULT_WAIT_SECONDS
        if (step.Selector) await waitSelector(page, step.Selector, seconds * 1000)
        else if (step.URL) await waitUrl(page, step.URL)
        else await settle(page)
      } else {
        throw new WebError(`${where}: unknown action ${String(step.Action)}`)
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      const prefix = message.startsWith('login step') ? '' : `${where} failed: `
      throw new WebError(prefix + message + (await pageExcerpt(page)))
    }
  }
  await settle(page)
  if (steps.length > 0 && (await page.passwordFieldVisible()))
    throw new WebError(
      `login as ${user} did not complete: the page still shows a password field.` +
        (await pageExcerpt(page))
    )
}
