import { createRouter, createWebHashHistory } from 'vue-router'
import { loadSession, session } from '../stores/session'
import HomeView from '../views/HomeView.vue'
import LoginView from '../views/LoginView.vue'
import SubjectView from '../views/SubjectView.vue'
import PaperListView from '../views/PaperListView.vue'
import PracticeView from '../views/PracticeView.vue'
import WrongView from '../views/WrongView.vue'
import FavoritesView from '../views/FavoritesView.vue'
import NotesView from '../views/NotesView.vue'
import QuestionDetailView from '../views/QuestionDetailView.vue'
import PlaceholderView from '../views/PlaceholderView.vue'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', name: 'home', component: HomeView, meta: { title: '没钱考什么公' } },
    { path: '/login', name: 'login', component: LoginView, meta: { title: '登录 · 没钱考什么公', public: true } },
    { path: '/subject/:name', name: 'subject', component: SubjectView, meta: { title: '题库' } },
    { path: '/source/:subject/:category', name: 'source', component: PaperListView, meta: { title: '试卷列表' } },
    { path: '/practice/:subject/:category?', name: 'practice', component: PracticeView, meta: { title: '刷题' } },
    { path: '/papers', name: 'papers', component: PlaceholderView, meta: { title: '智能组卷' } },
    { path: '/wrong', name: 'wrong', component: WrongView, meta: { title: '错题本' } },
    { path: '/favorites', name: 'favorites', component: FavoritesView, meta: { title: '收藏' } },
    { path: '/notes', name: 'notes', component: NotesView, meta: { title: '笔记' } },
    { path: '/question/:id', name: 'question', component: QuestionDetailView, meta: { title: '题目详情' } },
    { path: '/custom', name: 'custom', component: PlaceholderView, meta: { title: '自定义题库' } },
    { path: '/ai', name: 'ai', component: PlaceholderView, meta: { title: 'AI 设置' } },
  ],
})

router.beforeEach(async (to) => {
  if (!session.ready) await loadSession()
  if (!to.meta.public && !session.user) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && session.user) {
    return { name: 'home' }
  }
  return true
})

router.afterEach((to) => {
  document.title = String(to.meta.title || '没钱考什么公')
})

export default router
