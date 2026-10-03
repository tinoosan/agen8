import Workspace from "@/components/workspace";
import { requireChatGPTUser } from "./chatgpt-auth";
export const dynamic = "force-dynamic";
export default async function Home() {
  const user = await requireChatGPTUser("/");
  return <Workspace displayName={user.displayName} />;
}
