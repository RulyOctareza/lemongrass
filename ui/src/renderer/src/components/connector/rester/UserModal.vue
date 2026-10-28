<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { describeError } from '../../../vaultSession'
import { userHandle } from '../../../userHandle'
import type { VaultRester, VaultResterUser, VaultUserOverride } from '../../../../../preload/types'
import AppButton from '../../AppButton.vue'
import AppCheckbox from '../../AppCheckbox.vue'
import AppModal from '../../AppModal.vue'
import AppSegmented from '../../AppSegmented.vue'
import FieldRows from './FieldRows.vue'
import {
  effectiveUser,
  fieldDrafts,
  fieldsFromDrafts,
  plainCopy,
  tagList,
  type CommitRester,
  type FieldDraft
} from './resterDraft'

const props = defineProps<{
  rester: VaultRester
  // Null for a new user.
  user: VaultResterUser | null
  commit: CommitRester
}>()

const emit = defineEmits<{
  close: []
}>()

interface OverrideDraft {
  open: boolean
  token: string
  fields: FieldDraft[]
}

type TestStatus = 'idle' | 'testing' | 'ok' | 'error'

const original = props.user
const name = ref(original?.Name ?? '')
const tags = ref((original?.Tags ?? []).join(', '))
const isByot = ref(original ? original.Token !== '' : false)
const token = ref(original?.Token ?? '')
const fields = ref<FieldDraft[]>(
  original
    ? fieldDrafts(original.Fields, original.FieldTypes)
    : [{ key: '', value: '', type: 'text' }]
)
const environments = ref<string[]>([...(original?.Environments ?? [])])
const overrides = reactive<Record<string, OverrideDraft>>({})
for (const env of props.rester.Environments) {
  const o = original?.Overrides?.[env.Name]
  overrides[env.Name] = {
    open: false,
    token: o?.Token ?? '',
    fields: o ? fieldDrafts(o.Fields, o.FieldTypes) : []
  }
}

const saving = ref(false)
const saveError = ref('')

const otherNames = computed(() =>
  props.rester.Users.filter((u) => u.Name !== original?.Name).map((u) => u.Name)
)

const collidesWith = computed(() => {
  const handle = userHandle(name.value)
  if (handle === '') return null
  return otherNames.value.find((n) => userHandle(n) === handle) ?? null
})

function toggleEnvironment(env: string): void {
  if (environments.value.includes(env)) {
    environments.value = environments.value.filter((e) => e !== env)
    if (testEnv.value === env) testEnv.value = ''
  } else {
    environments.value = [...environments.value, env]
  }
}

function buildOverride(draft: OverrideDraft): VaultUserOverride | null {
  if (isByot.value) {
    return draft.token.trim() !== '' ? { Fields: null, FieldTypes: null, Token: draft.token } : null
  }
  const built = fieldsFromDrafts(draft.fields)
  if (Object.keys(built.Fields).length === 0) return null
  return { Fields: built.Fields, FieldTypes: built.FieldTypes, Token: '' }
}

// Overrides are kept only for environments the user is assigned to, as the vault requires.
function buildUser(): VaultResterUser {
  const built = isByot.value ? { Fields: {}, FieldTypes: null } : fieldsFromDrafts(fields.value)
  const userOverrides: Record<string, VaultUserOverride> = {}
  for (const env of environments.value) {
    const o = overrides[env] ? buildOverride(overrides[env]) : null
    if (o) userOverrides[env] = o
  }
  return {
    Name: name.value.trim(),
    Fields: built.Fields,
    FieldTypes: built.FieldTypes,
    Token: isByot.value ? token.value : '',
    Tags: tagList(tags.value),
    Environments: props.rester.Environments.map((e) => e.Name).filter((e) =>
      environments.value.includes(e)
    ),
    Overrides: Object.keys(userOverrides).length > 0 ? userOverrides : null
  }
}

const testEnvironments = computed(() =>
  props.rester.Environments.filter((e) => e.BaseURL !== '' && environments.value.includes(e.Name))
)
const testEnv = ref('')
const testEnvOptions = computed(() =>
  testEnvironments.value.map((e) => ({ value: e.Name, label: e.Name }))
)
const selectedTestEnv = computed(
  () =>
    testEnvironments.value.find((e) => e.Name === testEnv.value) ??
    testEnvironments.value[0] ??
    null
)
const testStatus = ref<TestStatus>('idle')
const testError = ref('')
const testResponseStatus = ref(0)
const testResponseBody = ref('')
// The login response expands inside this modal, since Escape closes every open AppModal at once.
const showResponse = ref(false)

// A result belongs to the credentials it tested, so editing them clears it and a failed test stops blocking Save.
watch(
  [fields, token, isByot, () => Object.values(overrides).map((o) => [o.token, o.fields])],
  () => {
    if (testStatus.value === 'testing') return
    testStatus.value = 'idle'
    showResponse.value = false
  },
  { deep: true }
)

async function testLogin(): Promise<void> {
  const env = selectedTestEnv.value
  if (!props.rester.API || !env) return
  testStatus.value = 'testing'
  testError.value = ''
  testResponseStatus.value = 0
  testResponseBody.value = ''
  showResponse.value = false
  try {
    const result = await window.api.vault.testResterLogin(
      plainCopy(props.rester.API),
      plainCopy(env),
      effectiveUser(buildUser(), env.Name)
    )
    if (result.error === '') {
      testStatus.value = 'ok'
      return
    }
    testStatus.value = 'error'
    testError.value = describeError(result.error)
    testResponseStatus.value = result.status
    testResponseBody.value = result.body
  } catch (err) {
    testStatus.value = 'error'
    testError.value = describeError(err)
  }
}

function formatResponseBody(body: string): string {
  try {
    return JSON.stringify(JSON.parse(body), null, 2)
  } catch {
    return body
  }
}

const canSave = computed(
  () =>
    name.value.trim() !== '' &&
    collidesWith.value === null &&
    testStatus.value !== 'error' &&
    testStatus.value !== 'testing' &&
    !saving.value
)

async function save(): Promise<void> {
  if (!canSave.value) return
  saving.value = true
  const user = buildUser()
  saveError.value = await props.commit((r) => {
    const index = original ? r.Users.findIndex((u) => u.Name === original.Name) : -1
    if (index === -1) r.Users.push(user)
    else r.Users[index] = user
  })
  saving.value = false
  if (saveError.value === '') emit('close')
}

async function removeUser(): Promise<void> {
  if (!original) return
  saving.value = true
  saveError.value = await props.commit((r) => {
    r.Users = r.Users.filter((u) => u.Name !== original.Name)
  })
  saving.value = false
  if (saveError.value === '') emit('close')
}
</script>

<template>
  <AppModal
    scope="app"
    size="md"
    :title="original ? `User ${original.Name}` : 'New user'"
    @close="emit('close')"
  >
    <div class="user-form">
      <div class="field-block">
        <label class="field-label" for="user-name">Name</label>
        <input id="user-name" v-model="name" type="text" placeholder="User name" autofocus />
        <p v-if="userHandle(name) !== '' && userHandle(name) !== name" class="hint">
          Models use this user as {{ userHandle(name) }}.
        </p>
        <p v-if="collidesWith !== null" class="error-text">
          Models would use the same handle, {{ userHandle(name) }}, for "{{ collidesWith }}". Rename
          one of them.
        </p>
      </div>

      <div class="field-block">
        <label class="field-label" for="user-tags">Tags</label>
        <input
          id="user-tags"
          v-model="tags"
          type="text"
          placeholder="Comma separated, e.g. tenant x, low level"
        />
      </div>

      <div class="field-block">
        <span class="field-label">Environments</span>
        <div class="radio-row">
          <AppCheckbox
            v-for="env in rester.Environments"
            :key="env.Name"
            :checked="environments.includes(env.Name)"
            @click="toggleEnvironment(env.Name)"
          >
            {{ env.Name }}
          </AppCheckbox>
        </div>
        <p v-if="environments.length === 0" class="hint">
          Unassigned. No channel can use this user until it has an environment.
        </p>
      </div>

      <div class="field-block">
        <AppCheckbox :checked="isByot" @click="isByot = !isByot">Bring your own token</AppCheckbox>
        <input v-if="isByot" v-model="token" type="text" placeholder="Paste the token" />
        <template v-else>
          <span class="field-label">Login fields</span>
          <p class="hint">Sent in the API login body and typed by the web login steps.</p>
          <FieldRows v-model="fields" add-label="+ Field" />
        </template>
      </div>

      <div class="field-block">
        <span id="user-test-env" class="field-label">Test login</span>
        <p v-if="!rester.API" class="hint">The API side is off, so there is no login to test.</p>
        <p v-else-if="testEnvironments.length === 0" class="hint">
          Assign an environment with an API base URL to test the login.
        </p>
        <template v-else>
          <AppSegmented
            v-if="testEnvironments.length > 1"
            :model-value="selectedTestEnv?.Name ?? ''"
            :options="testEnvOptions"
            aria-labelledby="user-test-env"
            @update:model-value="(v) => (testEnv = v)"
          />
          <div class="test-row">
            <AppButton :disabled="testStatus === 'testing'" @click="testLogin">
              {{
                testStatus === 'testing' ? 'Testing...' : `Test login on ${selectedTestEnv?.Name}`
              }}
            </AppButton>
            <span
              class="test-status"
              :class="{
                'test-status-ok': testStatus === 'ok',
                'test-status-error': testStatus === 'error'
              }"
            >
              <template v-if="testStatus === 'ok'">✓ OK</template>
              <template v-else-if="testStatus === 'error'">✗ {{ testError }}</template>
              <template v-else-if="testStatus === 'idle'">Not tested yet</template>
            </span>
            <button
              v-if="testStatus === 'error' && testResponseStatus > 0"
              type="button"
              class="response-detail-button"
              title="Show the login response"
              aria-label="Show the login response"
              @click="showResponse = !showResponse"
            >
              !
            </button>
          </div>
          <template v-if="showResponse">
            <p class="hint">The login endpoint answered with status {{ testResponseStatus }}.</p>
            <pre v-if="testResponseBody" class="response-body">{{
              formatResponseBody(testResponseBody)
            }}</pre>
            <p v-else class="hint">The response had no body.</p>
          </template>
        </template>
      </div>

      <div v-if="environments.length > 0" class="field-block">
        <span class="field-label">Overrides per environment</span>
        <p class="hint">
          {{
            isByot
              ? 'A token here replaces the token above in that environment.'
              : 'A field here replaces the field of the same name in that environment. The rest are inherited.'
          }}
        </p>
        <template v-for="env in rester.Environments" :key="env.Name">
          <div v-if="environments.includes(env.Name) && overrides[env.Name]" class="user-draft">
            <button
              type="button"
              class="settings-toggle"
              :aria-expanded="overrides[env.Name].open"
              @click="overrides[env.Name].open = !overrides[env.Name].open"
            >
              <span class="settings-chevron" :class="{ expanded: overrides[env.Name].open }"
                >▸</span
              >
              {{ env.Name }}
              <span class="settings-summary">
                {{
                  (
                    isByot
                      ? overrides[env.Name].token.trim() !== ''
                      : overrides[env.Name].fields.length > 0
                  )
                    ? 'Overridden'
                    : 'Inherits'
                }}
              </span>
            </button>
            <template v-if="overrides[env.Name].open">
              <input
                v-if="isByot"
                v-model="overrides[env.Name].token"
                type="text"
                :placeholder="`Token for ${env.Name}, blank to inherit`"
              />
              <FieldRows v-else v-model="overrides[env.Name].fields" add-label="+ Override field" />
            </template>
          </div>
        </template>
      </div>

      <p v-if="saveError" class="error-text">{{ saveError }}</p>
    </div>

    <template #footer>
      <AppButton v-if="original" variant="danger" :disabled="saving" @click="removeUser">
        Remove user
      </AppButton>
      <span class="footer-fill"></span>
      <AppButton @click="emit('close')">Cancel</AppButton>
      <AppButton
        variant="primary"
        :disabled="!canSave"
        :title="
          testStatus === 'error' ? 'The last test login failed. Test again before saving.' : ''
        "
        @click="save"
      >
        {{ saving ? 'Saving...' : 'Save' }}
      </AppButton>
    </template>
  </AppModal>
</template>

<style scoped>
.user-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-height: 0;
  /* AppModal's body does not scroll, so a long form scrolls itself. */
  overflow-y: auto;
}

.footer-fill {
  flex: 1;
}
</style>
