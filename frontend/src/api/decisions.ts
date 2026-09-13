import apiClient from './client'

export type DecisionChoice='approve'|'reject'|'abstain'
export interface DecisionElector { id?:string; agentId:number; agentName?:string; role?:string; weight:number; canVeto:boolean }
export interface DecisionVote { id:string; agentId:number; agentName?:string; choice:DecisionChoice; weight?:number; reason?:string; createdAt:string }
export interface DecisionResult { approveWeight:number; rejectWeight:number; abstainWeight:number; quorumMet:boolean; vetoed:boolean; explanation:string }
export interface CollectiveDecision { id:string; groupId:string; workflowRunId?:string; title:string; description?:string; quorumPercent:number; approvalPercent:number; timeoutAction:'none'|'escalate'|'transfer'; escalateAgentId?:number; deadlineAt?:string; status:'open'|'passed'|'rejected'|'no_quorum'|'timed_out'|'escalated'; electorate?:DecisionElector[]; votes?:DecisionVote[]; result?:DecisionResult; createdAt:string; closedAt?:string }
export interface DecisionAudit { id:string; action:string; actorId?:string; description?:string; createdAt:string }
export const decisionApi={
  list:(groupId?:string)=>apiClient.get('/api/v1/admin/decisions',{params:groupId?{groupId}:{}}),
  create:(data:Omit<CollectiveDecision,'id'|'status'|'createdAt'|'closedAt'|'votes'|'result'|'electorate'>&{electors:DecisionElector[]})=>apiClient.post('/api/v1/admin/decisions',data),
  get:(id:string)=>apiClient.get(`/api/v1/admin/decisions/${id}`),
  vote:(id:string,data:{agentId:number;choice:DecisionChoice;reason?:string})=>apiClient.post(`/api/v1/admin/decisions/${id}/votes`,data),
  close:(id:string)=>apiClient.post(`/api/v1/admin/decisions/${id}/close`),
  timeout:(id:string)=>apiClient.post(`/api/v1/admin/decisions/${id}/timeout`),
  audit:(id:string)=>apiClient.get(`/api/v1/admin/decisions/${id}/audit`),
}
