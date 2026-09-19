import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath } from 'node:url'

const projectFile = (path) => fileURLToPath(new URL(path, import.meta.url))
const repositoryRoot = projectFile('../')

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, repositoryRoot, '')
  const backendUrl = env.WEB_BACKEND_URL || 'http://127.0.0.1:8888'
  const allowedHosts = (env.WEB_ALLOWED_HOSTS || 'localhost,127.0.0.1')
    .split(',')
    .map((host) => host.trim())
    .filter(Boolean)

  return {
    envDir: repositoryRoot,
    plugins: [react()],
    resolve: {
      alias: [
        {
          find: /^mediabunny$/,
          replacement: projectFile(
            './src/features/video/media/mediabunnyCompat.js',
          ),
        },
        {
          find: /^mediabunny-official$/,
          replacement: projectFile(
            './node_modules/mediabunny/dist/modules/src/index.js',
          ),
        },
      ],
    },
    server: {
      allowedHosts,
      proxy: {
        '/api': backendUrl,
      },
    },
  }
})
