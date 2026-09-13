import { useMutation,useQuery,useQueryClient } from '@tanstack/react-query'
import { message } from 'antd'
import { unwrapResponse,parseApiError } from '@/api/client'
import { workflowApi,type Workflow,type WorkflowExecution,type WorkflowAudit } from '@/api/workflows'
import { decisionApi,type CollectiveDecision,type DecisionAudit } from '@/api/decisions'
const list=<T,>(data:T[]|{items?:T[];workflows?:T[];decisions?:T[]})=>Array.isArray(data)?data:data.items??data.workflows??data.decisions??[]
export const useWorkflows=()=>useQuery({queryKey:['workflows'],queryFn:async()=>list(unwrapResponse<Workflow[]>(await workflowApi.list()))})
export const useWorkflow=(id?:string)=>useQuery({queryKey:['workflows',id],queryFn:async()=>{const data=unwrapResponse<{workflow:Workflow;versions:Workflow['versions']}>(await workflowApi.get(id as string));return {...data.workflow,versions:data.versions}},enabled:Boolean(id)})
export const useWorkflowExecutions=(workflowId?:string)=>useQuery({queryKey:['workflow-executions','list',workflowId],queryFn:async()=>list(unwrapResponse<WorkflowExecution[]>(await workflowApi.executions(workflowId))),enabled:Boolean(workflowId),refetchInterval:3000})
export const useWorkflowExecution=(id?:string)=>useQuery({queryKey:['workflow-executions',id],queryFn:async()=>unwrapResponse<WorkflowExecution>(await workflowApi.execution(id as string)),enabled:Boolean(id),refetchInterval:3000})
export const useWorkflowAudit=(id?:string)=>useQuery({queryKey:['workflow-executions',id,'audit'],queryFn:async()=>list(unwrapResponse<WorkflowAudit[]>(await workflowApi.executionAudit(id as string))),enabled:Boolean(id)})
export const useDecisions=(groupId?:string)=>useQuery({queryKey:['decisions',groupId],queryFn:async()=>list(unwrapResponse<CollectiveDecision[]>(await decisionApi.list(groupId)))})
export const useDecision=(id?:string)=>useQuery({queryKey:['decisions','detail',id],queryFn:async()=>unwrapResponse<CollectiveDecision>(await decisionApi.get(id as string)),enabled:Boolean(id),refetchInterval:3000})
export const useDecisionAudit=(id?:string)=>useQuery({queryKey:['decisions','audit',id],queryFn:async()=>list(unwrapResponse<DecisionAudit[]>(await decisionApi.audit(id as string))),enabled:Boolean(id)})
export function useGovernanceAction<T>(fn:(input:T)=>Promise<unknown>,success:string){const qc=useQueryClient();return useMutation({mutationFn:fn,onSuccess:()=>{void qc.invalidateQueries({queryKey:['workflows']});void qc.invalidateQueries({queryKey:['workflow-executions']});void qc.invalidateQueries({queryKey:['decisions']});message.success(success)},onError:(e)=>message.error(parseApiError(e))})}
