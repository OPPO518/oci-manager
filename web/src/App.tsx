import { useEffect } from 'react'
import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { Layout } from './components/Layout'
import { Loading } from './components/ui'
import { setUnauthorizedHandler } from './lib/api'
import { AuthProvider, useAuth } from './lib/auth'
import { AccountProvider } from './lib/context'
import Accounts from './pages/Accounts'
import Capacity from './pages/Capacity'
import Cost from './pages/Cost'
import Grabber from './pages/Grabber'
import Identity from './pages/Identity'
import Instances from './pages/Instances'
import Login from './pages/Login'
import Network from './pages/Network'
import Overview from './pages/Overview'
import Settings from './pages/Settings'

/** 未登录时只渲染登录页，避免 AccountProvider 发出无谓的 401 请求 */
function Shell() {
  const { loading, authenticated, expire } = useAuth()

  useEffect(() => {
    setUnauthorizedHandler(expire)
    return () => setUnauthorizedHandler(null)
  }, [expire])

  if (loading) {
    return (
      <div className="flex min-h-full items-center justify-center py-24">
        <Loading text="正在检查登录状态…" />
      </div>
    )
  }

  if (!authenticated) {
    return <Login />
  }

  return (
    <AccountProvider>
      <BrowserRouter>
        <Layout>
          <Routes>
            <Route path="/" element={<Overview />} />
            <Route path="/instances" element={<Instances />} />
            <Route path="/grabber" element={<Grabber />} />
            <Route path="/capacity" element={<Capacity />} />
            <Route path="/cost" element={<Cost />} />
            <Route path="/accounts" element={<Accounts />} />
            <Route path="/settings" element={<Settings />} />
            <Route path="/network" element={<Network />} />
            <Route path="/identity" element={<Identity />} />
            <Route path="*" element={<Overview />} />
          </Routes>
        </Layout>
      </BrowserRouter>
    </AccountProvider>
  )
}

export default function App() {
  return (
    <AuthProvider>
      <Shell />
    </AuthProvider>
  )
}
