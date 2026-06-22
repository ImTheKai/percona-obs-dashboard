import type { Context } from '../types/api'

export const PPG_CONTEXT: Context = {
  label: 'PPG',
  apiBase: '/api/products/ppg',
  prefix: 'isv:percona:ppg',
}

export const RELEASES_CONTEXT: Context = {
  label: 'Releases',
  apiBase: '/api/releases/ppg',
  prefix: 'isv:percona:ppg:releases',
}
