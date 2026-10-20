import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildGraph } from './historyGraph.ts'

test('a straight history stays in one lane', () => {
  const graph = buildGraph([
    { hash: 'c', parents: ['b'] },
    { hash: 'b', parents: ['a'] },
    { hash: 'a', parents: [] }
  ])
  assert.equal(graph.lanes, 1)
  assert.deepEqual(
    graph.rows.map((row) => row.lane),
    [0, 0, 0]
  )
})

test('a merge opens a second lane and the older branch joins the shared parent', () => {
  // m merges f (feature) into b (main); both come from a.
  const graph = buildGraph([
    { hash: 'm', parents: ['b', 'f'] },
    { hash: 'f', parents: ['a'] },
    { hash: 'b', parents: ['a'] },
    { hash: 'a', parents: [] }
  ])
  assert.equal(graph.lanes, 2)
  assert.deepEqual(
    graph.rows.map((row) => row.lane),
    [0, 1, 0, 1]
  )
  // The merge commit sends one line down each lane.
  const merge = graph.rows[0].lines.map((line) => [line.fromLane, line.toLane, line.toY])
  assert.deepEqual(merge, [
    [0, 0, 1],
    [0, 1, 1]
  ])
  // Commit b ends its lane by joining the lane that already waits for their shared parent a.
  assert.ok(graph.rows[2].lines.some((line) => line.fromLane === 0 && line.toLane === 1))
  // The lane then ends at a, which sits on lane 1.
  assert.ok(graph.rows[3].lines.some((line) => line.fromLane === 1 && line.toLane === 1))
})

test('a branch tip that nothing points at starts a new lane', () => {
  const graph = buildGraph([
    { hash: 'x', parents: ['a'] },
    { hash: 'y', parents: ['a'] },
    { hash: 'a', parents: [] }
  ])
  assert.deepEqual(
    graph.rows.map((row) => row.lane),
    [0, 1, 0]
  )
  assert.equal(graph.lanes, 2)
})
