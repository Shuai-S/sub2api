import { apiClient } from './client'
import type { CustomMenuLocale, CustomMenuModalContent } from '@/types'

export async function getModalContent(id: string, locale: CustomMenuLocale = 'en'): Promise<CustomMenuModalContent> {
  const { data } = await apiClient.get<CustomMenuModalContent>(
    `/custom-menu-items/${encodeURIComponent(id)}/modal`,
    { params: { locale } },
  )
  return data
}

const customMenuAPI = {
  getModalContent,
}

export default customMenuAPI
