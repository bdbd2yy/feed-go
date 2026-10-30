import { useMutation, useQuery } from '@tanstack/react-query'
import * as social from '../api/social'
import { useAuth } from '../lib/auth'
import { queryClient } from '../lib/query'
import { notifyError } from '../components/ui/toast'
export function useSocial(id?: number) {
  const auth = useAuth()
  const scope = id || auth.claims?.account_id
  const followers = useQuery({
    queryKey: ['followers', scope],
    queryFn: () => social.getAllFollowers(id),
    enabled: auth.isLoggedIn && !!scope,
  })
  const following = useQuery({
    queryKey: ['following', scope],
    queryFn: () => social.getAllVloggers(id),
    enabled: auth.isLoggedIn && !!scope,
  })
  return { followers, following }
}
export function useFollow() {
  const { following } = useSocial()
  const mutation = useMutation({
    mutationFn: ({ id, followed }: { id: number; followed: boolean }) =>
      followed ? social.unfollow(id) : social.follow(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['following'] })
      void queryClient.invalidateQueries({ queryKey: ['followers'] })
    },
    onError: notifyError,
  })
  return {
    ...mutation,
    isFollowing: (id: number) => !!following.data?.vloggers.some((a) => a.id === id),
  }
}
