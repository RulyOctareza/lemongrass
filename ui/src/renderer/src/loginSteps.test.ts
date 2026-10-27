// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  buildStep,
  moveStep,
  removeStep,
  stepDrafts,
  type StepDraft
} from './components/connector/rester/loginSteps.ts'

function sample(): StepDraft[] {
  return stepDrafts([
    { Action: 'goto', URL: '/login', Selector: '', Field: '', TimeoutSeconds: 0 },
    { Action: 'fill', URL: '', Selector: '#email', Field: 'email', TimeoutSeconds: 0 },
    { Action: 'fill', URL: '', Selector: '#password', Field: 'password', TimeoutSeconds: 0 },
    { Action: 'click', URL: '', Selector: '#submit', Field: '', TimeoutSeconds: 0 },
    { Action: 'wait', URL: '/home*', Selector: '#home', Field: '', TimeoutSeconds: 7 }
  ])
}

function shape(steps: StepDraft[]): string[] {
  return steps.map((s) => `${s.Action} ${s.URL} ${s.Selector} ${s.Field} ${s.TimeoutSeconds}`)
}

test('removing a middle step keeps every other step and its id', () => {
  const steps = sample()
  const before = shape(steps)
  const ids = steps.map((s) => s.id)
  removeStep(steps, 2)
  assert.deepEqual(shape(steps), [before[0], before[1], before[3], before[4]])
  assert.deepEqual(
    steps.map((s) => s.id),
    [ids[0], ids[1], ids[3], ids[4]]
  )
})

test('every step gets its own id', () => {
  const ids = [...sample(), ...sample()].map((s) => s.id)
  assert.equal(new Set(ids).size, ids.length)
})

test('moving down lands the step in the gap it was dropped on', () => {
  const steps = sample()
  const before = shape(steps)
  moveStep(steps, 0, 3)
  assert.deepEqual(shape(steps), [before[1], before[2], before[0], before[3], before[4]])
})

test('moving up and to either end', () => {
  const steps = sample()
  const before = shape(steps)
  moveStep(steps, 4, 0)
  assert.deepEqual(shape(steps), [before[4], before[0], before[1], before[2], before[3]])
  moveStep(steps, 0, steps.length)
  assert.deepEqual(shape(steps), before)
})

test('dropping a step on either gap beside it changes nothing', () => {
  const steps = sample()
  const before = shape(steps)
  moveStep(steps, 2, 2)
  moveStep(steps, 2, 3)
  assert.deepEqual(shape(steps), before)
})

test('a built step keeps only the fields its action uses', () => {
  const [step] = stepDrafts([
    { Action: 'goto', URL: ' /login ', Selector: '#x', Field: 'email', TimeoutSeconds: 3 }
  ])
  assert.deepEqual(buildStep(step), {
    Action: 'goto',
    URL: '/login',
    Selector: '',
    Field: '',
    TimeoutSeconds: 0
  })
})
