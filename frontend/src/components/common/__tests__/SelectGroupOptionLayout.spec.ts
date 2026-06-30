import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick, ref } from 'vue'
import UiSelect from '@/components/common/Select.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

const longDescription =
  '接近max的claude，龙虾能吃，可用opus 4.7 4.8 均为 claude-opus-4-6 模型\n' +
  '反重力官方未提供 4.7 4.8 模型'

describe('Select group option layout', () => {
  it('keeps teleported dropdown within the viewport instead of expanding to content width', async () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 480 })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 720 })

    const Host = defineComponent({
      components: { UiSelect, GroupOptionItem },
      setup() {
        const groupId = ref<number | null>(null)
        return {
          groupId,
          options: [
            {
              value: 1,
              label: '很长很长的分组名称会包含没有空格的模型标识claude-opus-4-6-super-long-token',
              description: longDescription,
              rate: 0.8,
              platform: 'anthropic'
            }
          ]
        }
      },
      template: `
        <UiSelect
          v-model="groupId"
          :options="options"
          :searchable="true"
          placeholder="选择分组"
        >
          <template #option="{ option, selected }">
            <GroupOptionItem
              :name="option.label"
              :platform="option.platform"
              :rate-multiplier="option.rate"
              :description="option.description"
              :selected="selected"
            />
          </template>
        </UiSelect>
      `
    })

    const wrapper = mount(Host, {
      attachTo: document.body,
      global: {
        stubs: {
          Icon: true,
          PlatformIcon: true
        }
      }
    })
    const selectRoot = wrapper.findComponent(UiSelect).element as HTMLElement
    selectRoot.getBoundingClientRect = () =>
      ({
        bottom: 48,
        height: 40,
        left: 360,
        right: 460,
        top: 8,
        width: 100,
        x: 360,
        y: 8,
        toJSON: () => {}
      }) as DOMRect

    await wrapper.find('.select-trigger').trigger('click')
    await nextTick()

    const dropdown = document.body.querySelector<HTMLElement>('.select-dropdown-portal')
    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.width).toBe('360px')
    expect(dropdown?.style.left).toBe('108px')
    expect(dropdown?.className).not.toContain('w-max')
    expect(dropdown?.style.maxWidth).toBe('calc(100vw - 24px)')

    wrapper.unmount()
  })

  it('renders group name and description with newline-aware wrapping rules', () => {
    const wrapper = mount(GroupOptionItem, {
      props: {
        name: '牛图专用分组\nopus-sonnet-very-long-model-name-without-spaces',
        platform: 'openai',
        rateMultiplier: 1,
        description: longDescription
      },
      global: {
        stubs: {
          PlatformIcon: true
        }
      }
    })

    const name = wrapper.find('.groupOptionItemBadge span.truncate')
    const description = wrapper.find('.groupOptionItemDescription')

    expect(name.text()).toContain('牛图专用分组')
    expect(name.attributes('style')).toBeUndefined()
    expect(description.text()).toContain('反重力官方未提供')
    expect(description.classes()).toContain('groupOptionItemDescription')
  })
})
