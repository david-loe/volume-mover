import { Navigate, Route, Routes } from 'react-router-dom'
import { AppShell } from './components/AppShell'
import { DashboardPage } from './pages/DashboardPage'
import { HostsPage } from './pages/HostsPage'
import { VolumesPage } from './pages/VolumesPage'
import { VolumeDetailPage } from './pages/VolumeDetailPage'
import { TransferBuilderPage } from './pages/TransferBuilderPage'
import { JobsPage } from './pages/JobsPage'
import { JobDetailPage } from './pages/JobDetailPage'

export default function App() {
  return (
    <AppShell>
      <Routes>
        <Route path="/" element={<Navigate to="/app/dashboard" replace />} />
        <Route path="/app" element={<Navigate to="/app/dashboard" replace />} />
        <Route path="/app/dashboard" element={<DashboardPage />} />
        <Route path="/app/hosts" element={<HostsPage />} />
        <Route path="/app/volumes" element={<VolumesPage />} />
        <Route path="/app/volumes/:host/:name" element={<VolumeDetailPage />} />
        <Route path="/app/transfers/new" element={<TransferBuilderPage />} />
        <Route path="/app/transfers" element={<JobsPage />} />
        <Route path="/app/transfers/:jobId" element={<JobDetailPage />} />
      </Routes>
    </AppShell>
  )
}
