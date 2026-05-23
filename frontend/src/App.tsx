import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import AuthGate from "./components/AuthGate";
import Layout from "./components/Layout";
import RouteErrorBoundary from "./components/RouteErrorBoundary";
import StateShell from "./components/StateShell";
import { BrandingProvider } from "./branding";
import Dashboard from "./pages/Dashboard";
import PublicHome from "./pages/PublicHome";

const Accounts = lazy(() => import("./pages/Accounts"));
const Operations = lazy(() => import("./pages/Operations"));
const OperationsErrors = lazy(() => import("./pages/OperationsErrors"));
const Proxies = lazy(() => import("./pages/Proxies"));
const SchedulerBoard = lazy(() => import("./pages/SchedulerBoard"));
const Settings = lazy(() => import("./pages/Settings"));
const APISecurity = lazy(() => import("./pages/APISecurity"));
const Docs = lazy(() => import("./pages/Docs"));
const APIKeys = lazy(() => import("./pages/APIKeys"));
const Usage = lazy(() => import("./pages/Usage"));
const ImageStudio = lazy(() => import("./pages/ImageStudio"));
const PromptFilter = lazy(() => import("./pages/PromptFilter"));
const PublicModelChecker = lazy(() => import("./pages/PublicModelChecker"));

export default function App() {
  const isAdminShell = window.location.pathname.startsWith("/admin");

  if (!isAdminShell) {
    return (
      <BrandingProvider>
        <RouteErrorBoundary>
          <Routes>
            <Route path="/" element={<PublicHome />} />
            <Route
              path="/model-checker"
              element={
                <Suspense
                  fallback={
                    <StateShell variant="page" loading>
                      {null}
                    </StateShell>
                  }
                >
                  <PublicModelChecker />
                </Suspense>
              }
            />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </RouteErrorBoundary>
      </BrandingProvider>
    );
  }

  return (
    <BrandingProvider>
      <AuthGate>
        <Layout>
          <RouteErrorBoundary>
            <Suspense
              fallback={
                <StateShell variant="page" loading>
                  {null}
                </StateShell>
              }
            >
              <Routes>
                <Route path="/" element={<Dashboard />} />
                <Route path="/accounts" element={<Accounts />} />
                <Route path="/api-keys" element={<APIKeys />} />
                <Route path="/proxies" element={<Proxies />} />
                <Route
                  path="/images"
                  element={<Navigate to="/images/studio" replace />}
                />
                <Route path="/images/:view" element={<ImageStudio />} />
                <Route
                  path="/prompt-filter"
                  element={<Navigate to="/prompt-filter/overview" replace />}
                />
                <Route path="/prompt-filter/:view" element={<PromptFilter />} />
                <Route
                  path="/ops"
                  element={<Navigate to="/ops/overview" replace />}
                />
                <Route path="/ops/overview" element={<Operations />} />
                <Route path="/ops/errors" element={<OperationsErrors />} />
                <Route path="/ops/scheduler" element={<SchedulerBoard />} />
                <Route path="/usage" element={<Usage />} />
                <Route path="/api-security" element={<APISecurity />} />
                <Route path="/settings" element={<Settings />} />
                <Route path="/docs" element={<Docs />} />
                <Route
                  path="/guide"
                  element={<Navigate to="/docs" replace />}
                />
                <Route
                  path="/api-reference"
                  element={<Navigate to="/docs#model-api" replace />}
                />
              </Routes>
            </Suspense>
          </RouteErrorBoundary>
        </Layout>
      </AuthGate>
    </BrandingProvider>
  );
}
