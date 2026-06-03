import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import { useNavigation } from '../lib/routing'
import type {
  ManifestGetProjectYamlResult,
  ManifestUpdateProjectYamlResult,
  ManifestGetTemplateYamlResult,
  ManifestUpdateTemplateYamlResult,
} from '../lib/types'

// ── agen8.yaml ──────────────────────────────────────

export function useProjectYaml(projectRootOrEnabled?: string | boolean, enabled = true) {
  const { focusedProjectRoot } = useNavigation()
  const projectRoot = typeof projectRootOrEnabled === 'string' ? projectRootOrEnabled : focusedProjectRoot
  const queryEnabled = typeof projectRootOrEnabled === 'boolean' ? projectRootOrEnabled : enabled
  return useQuery<ManifestGetProjectYamlResult>({
    queryKey: ['manifest.getProjectYaml', projectRoot],
    queryFn: () => rpcCall<ManifestGetProjectYamlResult>('manifest.getProjectYaml', {
      ...(projectRoot ? { projectRoot } : {}),
    }),
    staleTime: 30_000,
    retry: 1,
    enabled: queryEnabled,
  })
}

export function useProjectYamlUpdate(projectRootOverride?: string | null) {
  const queryClient = useQueryClient()
  const { focusedProjectRoot } = useNavigation()
  const projectRoot = projectRootOverride ?? focusedProjectRoot
  return useMutation<ManifestUpdateProjectYamlResult, Error, { yaml: string }>({
    mutationFn: (params) =>
      rpcCall<ManifestUpdateProjectYamlResult>('manifest.updateProjectYaml', {
        ...params,
        ...(projectRoot ? { projectRoot } : {}),
      }),
    onSuccess: (data) => {
      queryClient.setQueryData(['manifest.getProjectYaml', projectRoot], {
        yaml: data.yaml,
        filePath: data.filePath,
      })
      queryClient.invalidateQueries({ queryKey: ['project.space.list'] })
    },
  })
}

// ── template YAML ───────────────────────────────────

export function useTemplateYaml(templateId: string | null, projectRoot?: string | null, enabled = true) {
  const nav = useNavigation()
  const root = projectRoot ?? nav.focusedProjectRoot
  return useQuery<ManifestGetTemplateYamlResult>({
    queryKey: ['manifest.getTemplateYaml', templateId, root],
    queryFn: () => rpcCall<ManifestGetTemplateYamlResult>('manifest.getTemplateYaml', {
      templateId,
      ...(root ? { projectRoot: root } : {}),
    }),
    staleTime: 30_000,
    retry: 1,
    enabled: enabled && !!templateId,
  })
}

export function useTemplateYamlUpdate() {
  const queryClient = useQueryClient()
  const { focusedProjectRoot } = useNavigation()
  return useMutation<ManifestUpdateTemplateYamlResult, Error, { templateId: string; yaml: string }>({
    mutationFn: (params) =>
      rpcCall<ManifestUpdateTemplateYamlResult>('manifest.updateTemplateYaml', {
        ...params,
        ...(focusedProjectRoot ? { projectRoot: focusedProjectRoot } : {}),
      }),
    onSuccess: (data) => {
      queryClient.setQueryData(['manifest.getTemplateYaml', data.templateId, focusedProjectRoot], {
        yaml: data.yaml,
        filePath: data.filePath,
        templateId: data.templateId,
      })
      queryClient.invalidateQueries({ queryKey: ['project.space.list'] })
    },
  })
}
