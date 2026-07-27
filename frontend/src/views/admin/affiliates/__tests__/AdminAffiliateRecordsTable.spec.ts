import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import AdminAffiliateRecordsTable from '../AdminAffiliateRecordsTable.vue'

const { listInviteRecords } = vi.hoisted(() => ({
  listInviteRecords: vi.fn().mockResolvedValue({
    items: [],
    total: 0,
    page: 1,
    page_size: 20,
    pages: 0,
  }),
}))

vi.mock('@/api/admin/affiliates', () => {
  const affiliatesAPI = {
    listInviteRecords,
    listRebateRecords: vi.fn(),
    listTransferRecords: vi.fn(),
    getUserOverview: vi.fn(),
  }
  return { default: affiliatesAPI, affiliatesAPI }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn() }),
}))

vi.mock('@/utils/apiError', () => ({
  extractI18nErrorMessage: () => 'error',
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const DataTableStub = defineComponent({
  name: 'DataTableStub',
  props: {
    columns: { type: Array, required: true },
    data: { type: Array, required: true },
  },
  setup() {
    return () => h('div', { 'data-testid': 'affiliate-records-table' })
  },
})

describe('AdminAffiliateRecordsTable invite columns', () => {
  it('shows sortable inviter and invitee registration reward columns', async () => {
    const wrapper = mount(AdminAffiliateRecordsTable, {
      props: { type: 'invites' },
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>',
          },
          DataTable: DataTableStub,
          Pagination: true,
          BaseDialog: true,
          Icon: true,
          OrderStatusBadge: true,
        },
      },
    })

    await flushPromises()

    const columns = wrapper.getComponent(DataTableStub).props('columns') as Array<{
      key: string
      sortable?: boolean
    }>
    expect(columns).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ key: 'inviter_registration_reward', sortable: true }),
        expect.objectContaining({ key: 'invitee_registration_reward', sortable: true }),
      ]),
    )
    expect(listInviteRecords).toHaveBeenCalledWith(
      expect.objectContaining({ sort_by: 'created_at', sort_order: 'desc' }),
    )
  })
})
