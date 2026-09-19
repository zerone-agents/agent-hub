import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { parseApiError, unwrapResponse } from '@/api/client'
import { relationTypeApi, type RelationTypePayload, type RelationTypeTemplate } from '@/api/relation-types'

export function useRelationTypes() { return useQuery<RelationTypeTemplate[]>({ queryKey: ['relation-types'], queryFn: async () => unwrapResponse(await relationTypeApi.list()) }) }
export function useCreateRelationType() { const qc=useQueryClient(); return useMutation({mutationFn:(data:RelationTypePayload)=>relationTypeApi.create(data),onSuccess:()=>{void qc.invalidateQueries({queryKey:['relation-types']});message.success('关系类型已创建')},onError:(e)=>message.error(parseApiError(e))}) }
export function useUpdateRelationType() { const qc=useQueryClient(); return useMutation({mutationFn:({name,data}:{name:string;data:RelationTypePayload})=>relationTypeApi.update(name,data),onSuccess:()=>{void qc.invalidateQueries({queryKey:['relation-types']});message.success('关系类型已更新')},onError:(e)=>message.error(parseApiError(e))}) }
export function useDeleteRelationType() { const qc=useQueryClient(); return useMutation({mutationFn:(name:string)=>relationTypeApi.delete(name),onSuccess:()=>{void qc.invalidateQueries({queryKey:['relation-types']});message.success('关系类型已删除')},onError:(e)=>message.error(parseApiError(e))}) }
