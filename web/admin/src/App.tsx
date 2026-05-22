import { Routes, Route, Navigate } from 'react-router-dom'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import Agents from './pages/Agents'
import AgentDetail from './pages/AgentDetail'
import BuiltinAgents from './pages/BuiltinAgents'
import Tasks from './pages/Tasks'
import TaskDetail from './pages/TaskDetail'
import Traces from './pages/Traces'
import TraceContext from './pages/TraceContext'
import Chat from './pages/Chat'
import Groups from './pages/Groups'
import GroupDetail from './pages/GroupDetail'

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<Dashboard />} />
        <Route path="/agents" element={<Agents />} />
        <Route path="/agents/:name" element={<AgentDetail />} />
        <Route path="/builtin-agents" element={<BuiltinAgents />} />
        <Route path="/groups" element={<Groups />} />
        <Route path="/groups/:id" element={<GroupDetail />} />
        <Route path="/tasks" element={<Tasks />} />
        <Route path="/tasks/:id" element={<TaskDetail />} />
        <Route path="/traces" element={<Traces />} />
        <Route path="/traces/context/:contextId" element={<TraceContext />} />
        <Route path="/chat/:agentName" element={<Chat />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
