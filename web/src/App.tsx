import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { Layout } from './components/Layout'
import { AccountProvider } from './lib/context'
import Accounts from './pages/Accounts'
import Cost from './pages/Cost'
import Grabber from './pages/Grabber'
import Instances from './pages/Instances'
import Overview from './pages/Overview'

export default function App() {
  return (
    <AccountProvider>
      <BrowserRouter>
        <Layout>
          <Routes>
            <Route path="/" element={<Overview />} />
            <Route path="/instances" element={<Instances />} />
            <Route path="/grabber" element={<Grabber />} />
            <Route path="/cost" element={<Cost />} />
            <Route path="/accounts" element={<Accounts />} />
            <Route path="*" element={<Overview />} />
          </Routes>
        </Layout>
      </BrowserRouter>
    </AccountProvider>
  )
}
