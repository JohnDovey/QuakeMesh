package net.quakemesh.android.ui

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import net.quakemesh.android.R
import net.quakemesh.android.mesh.MeshDiscovery
import java.text.DateFormat
import java.util.Date

class PeersAdapter : RecyclerView.Adapter<PeersAdapter.Holder>() {
    private var items: List<MeshDiscovery.Peer> = emptyList()
    private val timeFmt = DateFormat.getTimeInstance(DateFormat.SHORT)

    fun submit(peers: List<MeshDiscovery.Peer>) {
        items = peers
        notifyDataSetChanged()
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): Holder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_peer, parent, false)
        return Holder(view)
    }

    override fun onBindViewHolder(holder: Holder, position: Int) {
        holder.bind(items[position], timeFmt)
    }

    override fun getItemCount(): Int = items.size

    class Holder(itemView: View) : RecyclerView.ViewHolder(itemView) {
        private val kindView: TextView = itemView.findViewById(R.id.peer_kind)
        private val detailView: TextView = itemView.findViewById(R.id.peer_detail)
        private val seenView: TextView = itemView.findViewById(R.id.peer_seen)

        fun bind(peer: MeshDiscovery.Peer, timeFmt: DateFormat) {
            kindView.text = when (peer.kind) {
                MeshDiscovery.Kind.HUB -> itemView.context.getString(R.string.peer_kind_hub, peer.address)
                MeshDiscovery.Kind.NODE -> itemView.context.getString(R.string.peer_kind_node, peer.address)
            }
            val idShort = peer.nodeId.take(16) + if (peer.nodeId.length > 16) "…" else ""
            detailView.text = when (peer.kind) {
                MeshDiscovery.Kind.HUB -> itemView.context.getString(
                    R.string.peer_hub_detail,
                    idShort,
                    peer.heartbeatUrl ?: "",
                )
                MeshDiscovery.Kind.NODE -> {
                    val loc = if (peer.lat != null && peer.lon != null) {
                        itemView.context.getString(R.string.peer_node_location, peer.lat, peer.lon)
                    } else {
                        ""
                    }
                    itemView.context.getString(R.string.peer_node_detail, idShort, loc)
                }
            }
            seenView.text = itemView.context.getString(
                R.string.peer_last_seen,
                timeFmt.format(Date(peer.lastSeenMs)),
            )
        }
    }
}
