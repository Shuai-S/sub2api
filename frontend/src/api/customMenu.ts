import { apiClient } from './client'
import type { CustomMenuModalContent } from '@/types'

export async function getModalContent(id: string): Promise<CustomMenuModalContent> {
  const { data } = await apiClient.get<CustomMenuModalContent>(
    `/custom-menu-items/${encodeURIComponent(id)}/modal`,
  )
  return data
}

const customMenuAPI = {
  getModalContent,
}

export default customMenuAPI
